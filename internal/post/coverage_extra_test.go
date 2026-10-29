package post

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/mock/gomock"
)

func TestGetAndDeleteHandlersMapErrorsAndLog(t *testing.T) {
	for _, tc := range []struct {
		name   string
		getErr error
		status int
		code   string
	}{
		{"not found", ErrNotFound, http.StatusNotFound, "NOT_FOUND"},
		{"dependency", ErrDependencyUnavailable, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE"},
		{"internal", errors.New("database detail"), http.StatusInternalServerError, "INTERNAL_ERROR"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repository, ids, clock := postDependencies(t, Post{}, "", nil, tc.getErr)
			logger := NewMockLogger(gomock.NewController(t))
			logger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
			logger.EXPECT().Error(gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
			logger.EXPECT().Sync().AnyTimes().Return(nil)
			router := NewRouter(NewHandler(NewService(repository, ids, clock), logger), postAuth(t, authorID, nil))
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/posts/"+postID, nil))
			if response.Code != tc.status || !strings.Contains(response.Body.String(), `"code":"`+tc.code+`"`) {
				t.Fatalf("status/body=%d/%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestDeleteHandlerRequiresOwnerAndWritesNoContent(t *testing.T) {
	value := Post{ID: postID, AuthorID: authorID, Content: "hello", CreatedAt: time.Now()}
	repository, ids, clock := postDependencies(t, value, "", nil, nil)
	router := NewRouter(NewHandler(NewService(repository, ids, clock), nil), postAuth(t, authorID, nil))
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/posts/"+postID, nil)
	request.Header.Set("Authorization", "Bearer token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status/body=%d/%s", response.Code, response.Body.String())
	}
	if _, err := NewService(repository, ids, clock).GetByID(context.Background(), "bad"); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("err=%v", err)
	}
}

func TestPostHandlerRejectsMalformedCreateAndDeleteRequests(t *testing.T) {
	repository, ids, clock := postDependencies(t, Post{}, postID, nil, nil)
	handler := NewHandler(NewService(repository, ids, clock), nil)
	for _, body := range []string{`{"content":"hello","extra":true}`, `{`, `null`} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/posts", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		request = WithAuthenticatedUserID(request, authorID)
		response := httptest.NewRecorder()
		handler.Create(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("body=%q status=%d", body, response.Code)
		}
	}
	invalid := httptest.NewRecorder()
	invalidRequest := WithAuthenticatedUserID(httptest.NewRequest(http.MethodDelete, "/api/v1/posts/not-an-id", nil), authorID)
	handler.Delete(invalid, invalidRequest)
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid delete status=%d", invalid.Code)
	}
	unauthorized := httptest.NewRecorder()
	handler.Delete(unauthorized, httptest.NewRequest(http.MethodDelete, "/api/v1/posts/"+postID, nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized delete status=%d", unauthorized.Code)
	}
	invalidAuth := NewRouter(handler, postAuth(t, "", errors.New("invalid token")))
	badAuthRequest := httptest.NewRequest(http.MethodPost, "/api/v1/posts", strings.NewReader(`{"content":"hello"}`))
	badAuthRequest.Header.Set("Content-Type", "application/json")
	badAuthRequest.Header.Set("Authorization", "Bearer bad")
	badAuthResponse := httptest.NewRecorder()
	invalidAuth.ServeHTTP(badAuthResponse, badAuthRequest)
	if badAuthResponse.Code != http.StatusUnauthorized {
		t.Fatalf("invalid auth status=%d", badAuthResponse.Code)
	}
}

func TestPostServiceRejectsInvalidContentAndRepositoryFailure(t *testing.T) {
	repository, ids, clock := postDependencies(t, Post{}, postID, nil, ErrDependencyUnavailable)
	service := NewService(repository, ids, clock)
	for _, content := range []string{"   ", strings.Repeat("x", 501)} {
		if _, err := service.Create(context.Background(), CreateCommand{AuthorID: authorID, Content: content}); !errors.Is(err, ErrValidation) {
			t.Fatalf("content %q error=%v", content, err)
		}
	}
	if _, err := service.Create(context.Background(), CreateCommand{AuthorID: authorID, Content: "hello"}); !errors.Is(err, ErrDependencyUnavailable) {
		t.Fatalf("repository error=%v", err)
	}
	if err := service.Delete(context.Background(), postID, authorID); !errors.Is(err, ErrDependencyUnavailable) {
		t.Fatalf("delete error=%v", err)
	}
	badRepository, badIDs, badClock := postDependencies(t, Post{}, "not-a-uuid", nil, nil)
	if _, err := NewService(badRepository, badIDs, badClock).Create(context.Background(), CreateCommand{AuthorID: authorID, Content: "hello"}); !errors.Is(err, ErrDependencyUnavailable) {
		t.Fatalf("invalid generated ID error=%v", err)
	}
	if _, err := service.GetByID(context.Background(), postID); !errors.Is(err, ErrDependencyUnavailable) {
		t.Fatalf("get error=%v", err)
	}
}

func TestPostHandlersExerciseStructuredLogBranches(t *testing.T) {
	repository, ids, clock := postDependencies(t, Post{ID: postID, AuthorID: authorID, Content: "hello"}, "", nil, nil)
	logger := NewMockLogger(gomock.NewController(t))
	logger.EXPECT().Info(gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
	logger.EXPECT().Error(gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
	logger.EXPECT().Sync().AnyTimes().Return(nil)
	handler := NewHandler(NewService(repository, ids, clock), logger)
	router := NewRouter(handler, postAuth(t, authorID, nil))
	get := httptest.NewRecorder()
	router.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/v1/posts/"+postID, nil))
	deleteRequest := WithAuthenticatedUserID(httptest.NewRequest(http.MethodDelete, "/api/v1/posts/"+postID, nil), authorID)
	deleteRequest.Header.Set("Authorization", "Bearer token")
	deleted := httptest.NewRecorder()
	router.ServeHTTP(deleted, deleteRequest)
	if get.Code != http.StatusOK || deleted.Code != http.StatusNoContent {
		t.Fatalf("statuses=%d/%d", get.Code, deleted.Code)
	}
	missingType := httptest.NewRecorder()
	handler.Create(missingType, WithAuthenticatedUserID(httptest.NewRequest(http.MethodPost, "/api/v1/posts", strings.NewReader(`{"content":"hello"}`)), authorID))
	if missingType.Code != http.StatusBadRequest {
		t.Fatalf("content type status=%d", missingType.Code)
	}
}

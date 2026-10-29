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

const (
	postID   = "0192c5f4-7b2a-7abd-8def-1234567890ab"
	authorID = "0192c5f4-7b2a-7abc-8def-1234567890ab"
)

func postDependencies(t *testing.T, value Post, id string, idErr, repositoryErr error) (*MockRepository, *MockIDGenerator, *MockClock) {
	ctrl := gomock.NewController(t)
	repository := NewMockRepository(ctrl)
	ids := NewMockIDGenerator(ctrl)
	clock := NewMockClock(ctrl)
	repository.EXPECT().Create(gomock.Any(), gomock.Any()).AnyTimes().Return(repositoryErr)
	repository.EXPECT().GetByID(gomock.Any(), gomock.Any()).AnyTimes().Return(value, repositoryErr)
	repository.EXPECT().Delete(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().Return(repositoryErr)
	ids.EXPECT().New().AnyTimes().Return(id, idErr)
	clock.EXPECT().Now().AnyTimes().Return(time.Date(2026, 10, 18, 12, 35, 0, 123000000, time.UTC))
	return repository, ids, clock
}

func postAuth(t *testing.T, id string, err error) *MockTokenAuthenticator {
	auth := NewMockTokenAuthenticator(gomock.NewController(t))
	auth.EXPECT().Authenticate(gomock.Any(), gomock.Any()).AnyTimes().Return(id, err)
	return auth
}

func TestCreateTrimsUnicodeAndPersists(t *testing.T) {
	repository, ids, clock := postDependencies(t, Post{}, postID, nil, nil)
	value, err := NewService(repository, ids, clock).Create(context.Background(), CreateCommand{AuthorID: authorID, Content: "  Привет 🌍  "})
	if err != nil || value.Content != "Привет 🌍" || value.CreatedAtEpoch != 1792326900123 {
		t.Fatalf("value=%+v err=%v", value, err)
	}
}

func TestCreateValidationAndDependencyErrors(t *testing.T) {
	repository, ids, clock := postDependencies(t, Post{}, postID, nil, nil)
	if _, err := NewService(repository, ids, clock).Create(context.Background(), CreateCommand{AuthorID: "bad", Content: "hello"}); !errors.Is(err, ErrValidation) {
		t.Fatalf("err=%v", err)
	}
	repository2, ids2, clock2 := postDependencies(t, Post{}, "", errors.New("entropy"), nil)
	if _, err := NewService(repository2, ids2, clock2).Create(context.Background(), CreateCommand{AuthorID: authorID, Content: "hello"}); !errors.Is(err, ErrDependencyUnavailable) {
		t.Fatalf("err=%v", err)
	}
}

func TestRouterRequiresBearerAndCreatesPost(t *testing.T) {
	repository, ids, clock := postDependencies(t, Post{}, postID, nil, nil)
	router := NewRouter(NewHandler(NewService(repository, ids, clock), nil), postAuth(t, authorID, nil))
	for _, authorization := range []string{"", "Basic token", "Bearer   "} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/posts", strings.NewReader(`{"content":"hello"}`))
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Authorization", authorization)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("authorization %q status=%d", authorization, response.Code)
		}
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/posts", strings.NewReader(`{"content":" hello "}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer token")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestGetByIDAndDeleteEnforceContract(t *testing.T) {
	value := Post{ID: postID, AuthorID: authorID, Content: "hello", CreatedAt: time.Date(2026, 10, 18, 12, 35, 0, 0, time.UTC)}
	repository, ids, clock := postDependencies(t, value, "", nil, nil)
	service := NewService(repository, ids, clock)
	if _, err := service.GetByID(context.Background(), "bad"); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("err=%v", err)
	}
	if err := service.Delete(context.Background(), postID, "other"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("err=%v", err)
	}
	if err := service.Delete(context.Background(), postID, authorID); err != nil {
		t.Fatalf("err=%v", err)
	}
}

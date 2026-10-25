package post

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeRepository struct {
	value       Post
	err         error
	getErr      error
	deleteErr   error
	id          string
	deleteID    string
	deleteOwner string
}

func (f *fakeRepository) Create(_ context.Context, value Post) error { f.value = value; return f.err }
func (f *fakeRepository) GetByID(_ context.Context, id string) (Post, error) {
	f.id = id
	if f.getErr != nil {
		return Post{}, f.getErr
	}
	return f.value, f.err
}
func (f *fakeRepository) Delete(_ context.Context, id, ownerID string) error {
	f.deleteID, f.deleteOwner = id, ownerID
	if f.deleteErr != nil {
		return f.deleteErr
	}
	return f.err
}

type fakeIDs struct {
	value string
	err   error
}

func (f fakeIDs) New() (string, error) { return f.value, f.err }

type fakeClock struct{ value time.Time }

func (f fakeClock) Now() time.Time { return f.value }

func TestCreateTrimsAndCountsUnicode(t *testing.T) {
	repo := &fakeRepository{}
	service := NewService(repo, fakeIDs{value: "0192c5f4-7b2a-7abd-8def-1234567890ab"}, fakeClock{value: time.Date(2026, 10, 18, 12, 35, 0, 123000000, time.UTC)})
	value, err := service.Create(context.Background(), CreateCommand{AuthorID: "0192c5f4-7b2a-7abc-8def-1234567890ab", Content: "  Привет 🌍  "})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if value.Content != "Привет 🌍" || repo.value.Content != value.Content {
		t.Fatalf("content = %q, want trimmed content", value.Content)
	}
	if value.CreatedAtEpoch != 1792326900123 {
		t.Fatalf("createdAtEpoch = %d", value.CreatedAtEpoch)
	}
}

func TestCreateRejectsEmptyAndMoreThan500CodePoints(t *testing.T) {
	service := NewService(&fakeRepository{}, fakeIDs{value: "0192c5f4-7b2a-7abd-8def-1234567890ab"}, fakeClock{value: time.Now()})
	for _, content := range []string{"x", strings.Repeat("🙂", 500)} {
		if _, err := service.Create(context.Background(), CreateCommand{AuthorID: "0192c5f4-7b2a-7abc-8def-1234567890ab", Content: content}); err != nil {
			t.Errorf("boundary content length %d: error = %v", len([]rune(content)), err)
		}
	}
	for _, content := range []string{" 	\n ", strings.Repeat("界", 501)} {
		if _, err := service.Create(context.Background(), CreateCommand{AuthorID: "0192c5f4-7b2a-7abc-8def-1234567890ab", Content: content}); !errors.Is(err, ErrValidation) {
			t.Errorf("error = %v, want ErrValidation", err)
		}
	}
}

func TestHandlerAuthenticationAndValidation(t *testing.T) {
	service := NewService(&fakeRepository{}, fakeIDs{value: "0192c5f4-7b2a-7abd-8def-1234567890ab"}, fakeClock{value: time.Now()})
	handler := NewHandler(service, nil)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/posts", strings.NewReader(`{"content":"hello"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.Create(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d", response.Code)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/posts", strings.NewReader(`{"content":"   "}`))
	request.Header.Set("Content-Type", "application/json")
	request = WithAuthenticatedUserID(request, "0192c5f4-7b2a-7abc-8def-1234567890ab")
	response = httptest.NewRecorder()
	handler.Create(response, request)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid content status = %d", response.Code)
	}
}

type fakeAuthenticator struct {
	id  string
	err error
}

func (f fakeAuthenticator) Authenticate(context.Context, string) (string, error) {
	return f.id, f.err
}

func TestRouterAuthenticationAndSuccessResponse(t *testing.T) {
	clock := fakeClock{value: time.Date(2026, 10, 18, 12, 35, 0, 123000000, time.UTC)}
	repo := &fakeRepository{}
	service := NewService(repo, fakeIDs{value: "0192c5f4-7b2a-7abd-8def-1234567890ab"}, clock)
	handler := NewHandler(service, nil)
	router := NewRouter(handler, fakeAuthenticator{id: "0192c5f4-7b2a-7abc-8def-1234567890ab"})

	for _, tc := range []struct {
		name       string
		authorize  string
		wantStatus int
		wantCode   string
	}{
		{name: "missing header", wantStatus: http.StatusUnauthorized, wantCode: "UNAUTHORIZED"},
		{name: "wrong scheme", authorize: "Basic token", wantStatus: http.StatusUnauthorized, wantCode: "UNAUTHORIZED"},
		{name: "empty bearer", authorize: "Bearer   ", wantStatus: http.StatusUnauthorized, wantCode: "UNAUTHORIZED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/v1/posts", strings.NewReader(`{"content":"hello"}`))
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Authorization", tc.authorize)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if w.Code != tc.wantStatus || !strings.Contains(w.Body.String(), `"code":"`+tc.wantCode+`"`) {
				t.Fatalf("status/body = %d/%s", w.Code, w.Body.String())
			}
		})
	}

	r := httptest.NewRequest(http.MethodPost, "/api/v1/posts", strings.NewReader(`{"content":"  hello  "}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("success status = %d, body = %s", w.Code, w.Body.String())
	}
	var response Response
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("decode success response: %v", err)
	}
	if response.Content != "hello" || response.AuthorID != "0192c5f4-7b2a-7abc-8def-1234567890ab" {
		t.Fatalf("response = %+v", response)
	}
}

func TestRouterRejectsInvalidToken(t *testing.T) {
	handler := NewHandler(NewService(&fakeRepository{}, fakeIDs{value: "0192c5f4-7b2a-7abd-8def-1234567890ab"}, fakeClock{value: time.Now()}), nil)
	router := NewRouter(handler, fakeAuthenticator{err: errors.New("invalid token")})
	r := httptest.NewRequest(http.MethodPost, "/api/v1/posts", strings.NewReader(`{"content":"hello"}`))
	r.Header.Set("Authorization", "Bearer bad")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized || !strings.Contains(w.Body.String(), `"code":"INVALID_TOKEN"`) {
		t.Fatalf("status/body = %d/%s", w.Code, w.Body.String())
	}
}

func TestHandlerRejectsStrictInvalidBodies(t *testing.T) {
	service := NewService(&fakeRepository{}, fakeIDs{value: "0192c5f4-7b2a-7abd-8def-1234567890ab"}, fakeClock{value: time.Now()})
	handler := NewHandler(service, nil)
	for _, body := range []string{
		`{"content":"hello","extra":true}`,
		`{"content":"hello","content":"again"}`,
		`{"content":"hello"}{}`,
		`null`,
		`{"content":"hello"} 42`,
	} {
		r := httptest.NewRequest(http.MethodPost, "/api/v1/posts", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r = WithAuthenticatedUserID(r, "0192c5f4-7b2a-7abc-8def-1234567890ab")
		w := httptest.NewRecorder()
		handler.Create(w, r)
		if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"code":"INVALID_JSON"`) {
			t.Errorf("body %q: status/body = %d/%s", body, w.Code, w.Body.String())
		}
	}

	r := httptest.NewRequest(http.MethodPost, "/api/v1/posts", strings.NewReader(`{"content":"hello"}`))
	r = WithAuthenticatedUserID(r, "0192c5f4-7b2a-7abc-8def-1234567890ab")
	w := httptest.NewRecorder()
	handler.Create(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing content type status = %d", w.Code)
	}

	r = httptest.NewRequest(http.MethodPost, "/api/v1/posts", strings.NewReader(`{"content":"hello"}`))
	r.Header.Set("Content-Type", "text/plain")
	r = WithAuthenticatedUserID(r, "0192c5f4-7b2a-7abc-8def-1234567890ab")
	w = httptest.NewRecorder()
	handler.Create(w, r)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), `"code":"INVALID_JSON"`) {
		t.Fatalf("wrong content type status/body = %d/%s", w.Code, w.Body.String())
	}
}

func TestServiceTranslatesIDAndRepositoryFailures(t *testing.T) {
	validAuthor := "0192c5f4-7b2a-7abc-8def-1234567890ab"
	for _, tc := range []struct {
		name string
		ids  fakeIDs
		repo error
	}{
		{name: "id generator failure", ids: fakeIDs{err: errors.New("entropy failure")}},
		{name: "invalid generated id", ids: fakeIDs{value: "not-a-uuid"}},
		{name: "repository failure", ids: fakeIDs{value: "0192c5f4-7b2a-7abd-8def-1234567890ab"}, repo: ErrDependencyUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewService(&fakeRepository{err: tc.repo}, tc.ids, fakeClock{value: time.Now()}).Create(context.Background(), CreateCommand{AuthorID: validAuthor, Content: "hello"})
			if tc.repo != nil && !errors.Is(err, tc.repo) {
				t.Fatalf("error = %v, want repository error", err)
			}
			if tc.repo == nil && !errors.Is(err, ErrDependencyUnavailable) {
				t.Fatalf("error = %v, want ErrDependencyUnavailable", err)
			}
		})
	}
}

func TestHandlerMapsRepositoryFailureToServiceUnavailable(t *testing.T) {
	handler := NewHandler(NewService(&fakeRepository{err: ErrDependencyUnavailable}, fakeIDs{value: "0192c5f4-7b2a-7abd-8def-1234567890ab"}, fakeClock{value: time.Now()}), nil)
	r := httptest.NewRequest(http.MethodPost, "/api/v1/posts", strings.NewReader(`{"content":"hello"}`))
	r.Header.Set("Content-Type", "application/json")
	r = WithAuthenticatedUserID(r, "0192c5f4-7b2a-7abc-8def-1234567890ab")
	w := httptest.NewRecorder()
	handler.Create(w, r)
	if w.Code != http.StatusServiceUnavailable || strings.Contains(w.Body.String(), "dependency unavailable") || !strings.Contains(w.Body.String(), `"code":"SERVICE_UNAVAILABLE"`) {
		t.Fatalf("status/body = %d/%s", w.Code, w.Body.String())
	}
}

func TestGetByIDValidatesIDBeforeRepository(t *testing.T) {
	repo := &fakeRepository{value: Post{ID: "0192c5f4-7b2a-7abd-8def-1234567890ab"}}
	service := NewService(repo, fakeIDs{}, fakeClock{})
	if _, err := service.GetByID(context.Background(), "0192c5f4-7b2a-6abd-8def-1234567890ab"); !errors.Is(err, ErrInvalidID) {
		t.Fatalf("error = %v, want ErrInvalidID", err)
	}
	if repo.id != "" {
		t.Fatalf("repository called with invalid id %q", repo.id)
	}

	validID := "0192c5f4-7b2a-7abd-8def-1234567890ab"
	if _, err := service.GetByID(context.Background(), validID); err != nil {
		t.Fatalf("GetByID() error = %v", err)
	}
	if repo.id != validID {
		t.Fatalf("repository id = %q, want %q", repo.id, validID)
	}
}

func TestGetByIDRejectsNonCanonicalUUIDv7AtEndpoint(t *testing.T) {
	validID := "0192c5f4-7b2a-7abd-8def-1234567890ab"
	for _, tc := range []struct {
		name string
		id   string
	}{
		{name: "uppercase UUID", id: "0192C5F4-7B2A-7ABD-8DEF-1234567890AB"},
		{name: "numeric ID", id: "1234567890"},
		{name: "malformed UUID structure", id: "0192c5f4-7b2a-7abd-8def-1234567890"},
		{name: "invalid variant", id: "0192c5f4-7b2a-7abd-cdef-1234567890ab"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepository{value: Post{ID: validID}}
			router := NewRouter(NewHandler(NewService(repo, fakeIDs{}, fakeClock{}), nil), fakeAuthenticator{})
			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/posts/"+tc.id, nil))

			if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), `"code":"VALIDATION_FAILED"`) {
				t.Fatalf("status/body = %d/%s", response.Code, response.Body.String())
			}
			if repo.id != "" {
				t.Fatalf("repository called with invalid id %q", repo.id)
			}
		})
	}
}

func TestGetByIDRouterReturnsPostDTOWithoutAuthentication(t *testing.T) {
	postValue := Post{ID: "0192c5f4-7b2a-7abd-8def-1234567890ab", AuthorID: "0192c5f4-7b2a-7abc-8def-1234567890ab", Content: "Hello from GET", CreatedAt: time.Date(2026, 10, 18, 12, 35, 0, 0, time.UTC), CreatedAtEpoch: 1792326900000}
	handler := NewHandler(NewService(&fakeRepository{value: postValue}, fakeIDs{}, fakeClock{}), nil)
	router := NewRouter(handler, fakeAuthenticator{})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/posts/"+postValue.ID, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body) != 4 || body["id"] != postValue.ID || body["authorId"] != postValue.AuthorID || body["content"] != postValue.Content || body["createdAt"] != "2026-10-18T12:35:00Z" {
		t.Fatalf("response DTO = %#v", body)
	}
}

func TestGetByIDRouterPreservesMillisecondDTOPrecision(t *testing.T) {
	createdAt := time.Date(2026, 10, 18, 12, 35, 0, 123000000, time.UTC)
	value := Post{ID: "0192c5f4-7b2a-7abd-8def-1234567890ab", AuthorID: "0192c5f4-7b2a-7abc-8def-1234567890ab", Content: "millisecond post", CreatedAt: createdAt, CreatedAtEpoch: createdAt.UnixMilli()}
	router := NewRouter(NewHandler(NewService(&fakeRepository{value: value}, fakeIDs{}, fakeClock{}), nil), fakeAuthenticator{})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/posts/"+value.ID, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body Response
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.CreatedAt != "2026-10-18T12:35:00.123Z" {
		t.Fatalf("createdAt = %q, want millisecond precision", body.CreatedAt)
	}
}

func TestGetByIDMapsDomainErrorsAndDoesNotLeakDetails(t *testing.T) {
	validID := "0192c5f4-7b2a-7abd-8def-1234567890ab"
	for _, tc := range []struct {
		name       string
		err        error
		path       string
		wantStatus int
		wantCode   string
		leak       string
	}{
		{name: "invalid id", path: "/api/v1/posts/not-an-id", wantStatus: http.StatusUnprocessableEntity, wantCode: "VALIDATION_FAILED"},
		{name: "missing post", err: ErrNotFound, path: "/api/v1/posts/" + validID, wantStatus: http.StatusNotFound, wantCode: "NOT_FOUND"},
		{name: "unexpected dependency failure", err: errors.New("DynamoDB connection refused"), path: "/api/v1/posts/" + validID, wantStatus: http.StatusInternalServerError, wantCode: "INTERNAL_ERROR", leak: "DynamoDB connection refused"},
		{name: "translated dependency failure", err: ErrDependencyUnavailable, path: "/api/v1/posts/" + validID, wantStatus: http.StatusServiceUnavailable, wantCode: "SERVICE_UNAVAILABLE", leak: "dependency unavailable"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewHandler(NewService(&fakeRepository{err: tc.err}, fakeIDs{}, fakeClock{}), nil)
			response := httptest.NewRecorder()
			NewRouter(handler, fakeAuthenticator{}).ServeHTTP(response, httptest.NewRequest(http.MethodGet, tc.path, nil))
			if response.Code != tc.wantStatus || !strings.Contains(response.Body.String(), `"code":"`+tc.wantCode+`"`) {
				t.Fatalf("status/body = %d/%s", response.Code, response.Body.String())
			}
			if tc.leak != "" && strings.Contains(response.Body.String(), tc.leak) {
				t.Fatalf("response leaked infrastructure detail: %s", response.Body.String())
			}
		})
	}
}

func TestDeleteServiceValidatesOwnershipAndDelegatesOnlyToOwner(t *testing.T) {
	const postID = "0192c5f4-7b2a-7abd-8def-1234567890ab"
	const ownerID = "0192c5f4-7b2a-7abc-8def-1234567890ab"
	for _, tc := range []struct {
		name       string
		id         string
		owner      string
		value      Post
		getErr     error
		deleteErr  error
		wantErr    error
		wantDelete bool
	}{
		{name: "invalid UUIDv7", id: "not-a-uuid", owner: ownerID, value: Post{ID: postID, AuthorID: ownerID}, wantErr: ErrInvalidID},
		{name: "missing post", id: postID, owner: ownerID, getErr: ErrNotFound, wantErr: ErrNotFound},
		{name: "non-owner", id: postID, owner: "0192c5f4-7b2a-7abc-8def-1234567890ac", value: Post{ID: postID, AuthorID: ownerID}, wantErr: ErrForbidden},
		{name: "owner dependency failure", id: postID, owner: ownerID, value: Post{ID: postID, AuthorID: ownerID}, deleteErr: ErrDependencyUnavailable, wantErr: ErrDependencyUnavailable, wantDelete: true},
		{name: "owner success", id: postID, owner: ownerID, value: Post{ID: postID, AuthorID: ownerID}, wantDelete: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepository{value: tc.value, getErr: tc.getErr, deleteErr: tc.deleteErr}
			err := NewService(repo, fakeIDs{}, fakeClock{}).Delete(context.Background(), tc.id, tc.owner)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("Delete() error = %v, want %v", err, tc.wantErr)
			}
			if (repo.deleteID != "") != tc.wantDelete {
				t.Fatalf("delete called = %t, want %t", repo.deleteID != "", tc.wantDelete)
			}
			if tc.wantDelete && (repo.deleteID != tc.id || repo.deleteOwner != tc.owner) {
				t.Fatalf("delete arguments = %q/%q", repo.deleteID, repo.deleteOwner)
			}
		})
	}
}

func TestDeleteHandlerMapsContractResponsesAndDoesNotLeakInfrastructureErrors(t *testing.T) {
	const postID = "0192c5f4-7b2a-7abd-8def-1234567890ab"
	const ownerID = "0192c5f4-7b2a-7abc-8def-1234567890ab"
	for _, tc := range []struct {
		name       string
		repo       *fakeRepository
		wantStatus int
		wantCode   string
		leak       string
	}{
		{name: "unauthenticated", repo: &fakeRepository{}, wantStatus: http.StatusUnauthorized, wantCode: "UNAUTHORIZED"},
		{name: "non-owner", repo: &fakeRepository{value: Post{ID: postID, AuthorID: "0192c5f4-7b2a-7abc-8def-1234567890ac"}}, wantStatus: http.StatusForbidden, wantCode: "FORBIDDEN"},
		{name: "missing", repo: &fakeRepository{getErr: ErrNotFound}, wantStatus: http.StatusNotFound, wantCode: "NOT_FOUND"},
		{name: "infrastructure error", repo: &fakeRepository{getErr: errors.New("DynamoDB connection refused")}, wantStatus: http.StatusInternalServerError, wantCode: "INTERNAL_ERROR", leak: "DynamoDB connection refused"},
		{name: "dependency unavailable", repo: &fakeRepository{getErr: ErrDependencyUnavailable}, wantStatus: http.StatusServiceUnavailable, wantCode: "SERVICE_UNAVAILABLE", leak: "dependency unavailable"},
		{name: "owner success", repo: &fakeRepository{value: Post{ID: postID, AuthorID: ownerID}}, wantStatus: http.StatusNoContent},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := NewHandler(NewService(tc.repo, fakeIDs{}, fakeClock{}), nil)
			router := NewRouter(handler, fakeAuthenticator{id: ownerID})
			request := httptest.NewRequest(http.MethodDelete, "/api/v1/posts/"+postID, nil)
			if tc.name != "unauthenticated" {
				request.Header.Set("Authorization", "Bearer token")
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.wantStatus || (tc.wantCode != "" && !strings.Contains(response.Body.String(), `"code":"`+tc.wantCode+`"`)) {
				t.Fatalf("status/body = %d/%s", response.Code, response.Body.String())
			}
			if tc.wantStatus == http.StatusNoContent && response.Body.Len() != 0 {
				t.Fatalf("204 response body = %q", response.Body.String())
			}
			if tc.leak != "" && strings.Contains(response.Body.String(), tc.leak) {
				t.Fatalf("response leaked infrastructure detail: %s", response.Body.String())
			}
		})
	}
}

func TestDeleteHandlerRejectsInvalidUUIDv7BeforeRepository(t *testing.T) {
	const ownerID = "0192c5f4-7b2a-7abc-8def-1234567890ab"
	repo := &fakeRepository{value: Post{ID: "0192c5f4-7b2a-7abd-8def-1234567890ab", AuthorID: ownerID}}
	router := NewRouter(NewHandler(NewService(repo, fakeIDs{}, fakeClock{}), nil), fakeAuthenticator{id: ownerID})
	for _, id := range []string{"0192C5F4-7B2A-7ABD-8DEF-1234567890AB", "0192c5f4-7b2a-6abd-8def-1234567890ab", "1234567890"} {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodDelete, "/api/v1/posts/"+id, nil)
		request.Header.Set("Authorization", "Bearer token")
		router.ServeHTTP(response, request)
		if response.Code != http.StatusUnprocessableEntity || !strings.Contains(response.Body.String(), `"code":"VALIDATION_FAILED"`) {
			t.Fatalf("id %q status/body = %d/%s", id, response.Code, response.Body.String())
		}
	}
	if repo.id != "" || repo.deleteID != "" {
		t.Fatalf("repository called for invalid ID: get=%q delete=%q", repo.id, repo.deleteID)
	}
}

func TestDeleteHandlerLogsCompletedAndFailedRequests(t *testing.T) {
	const postID = "0192c5f4-7b2a-7abd-8def-1234567890ab"
	const ownerID = "0192c5f4-7b2a-7abc-8def-1234567890ab"
	for _, tc := range []struct {
		name       string
		repo       *fakeRepository
		wantStatus int
		wantInfo   string
		wantError  string
	}{
		{name: "success", repo: &fakeRepository{value: Post{ID: postID, AuthorID: ownerID}}, wantStatus: http.StatusNoContent, wantInfo: "post deletion completed"},
		{name: "failure", repo: &fakeRepository{getErr: ErrNotFound}, wantStatus: http.StatusNotFound, wantError: "post deletion failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logger := &captureLogger{}
			handler := NewHandler(NewService(tc.repo, fakeIDs{}, fakeClock{}), logger)
			request := httptest.NewRequest(http.MethodDelete, "/api/v1/posts/"+postID, nil)
			request.Header.Set("Authorization", "Bearer token")
			response := httptest.NewRecorder()
			NewRouter(handler, fakeAuthenticator{id: ownerID}).ServeHTTP(response, request)

			if response.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, tc.wantStatus)
			}
			if logger.infoMessage != tc.wantInfo || logger.errorMessage != tc.wantError {
				t.Fatalf("log messages = info %q/error %q, want info %q/error %q", logger.infoMessage, logger.errorMessage, tc.wantInfo, tc.wantError)
			}
			fields := logger.infoFields
			if tc.wantError != "" {
				fields = logger.errorFields
			}
			if !hasLogField(fields, "post_id", postID) {
				t.Fatalf("log fields = %#v, missing post_id", fields)
			}
		})
	}
}

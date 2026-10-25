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
	value Post
	err   error
}

func (f *fakeRepository) Create(_ context.Context, value Post) error { f.value = value; return f.err }

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

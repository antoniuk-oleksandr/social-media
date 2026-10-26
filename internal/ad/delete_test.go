package ad

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/antoniuk-oleksandr/social-media/internal/auth"
)

func TestDeleteAdvertisement(t *testing.T) {
	const adID = "0192f8a0-7b00-7000-8000-000000000001"
	const adminID = "0192f8a0-7b00-7000-8000-000000000002"

	t.Run("service validates the UUID and delegates deletion", func(t *testing.T) {
		repository := &fakeRepository{}
		service := NewService(repository, fakeIDs{}, fakeClock{})
		if err := service.Delete(context.Background(), "not-an-id"); !errors.Is(err, ErrInvalidID) {
			t.Fatalf("Delete() error = %v, want %v", err, ErrInvalidID)
		}
		if repository.deleteID != "" {
			t.Fatal("repository was called for an invalid ID")
		}
		if err := service.Delete(context.Background(), adID); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}
		if repository.deleteID != adID {
			t.Fatalf("deleted ID = %q, want %q", repository.deleteID, adID)
		}
	})

	t.Run("handler maps delete outcomes", func(t *testing.T) {
		for _, test := range []struct {
			name       string
			path       string
			repository *fakeRepository
			status     int
			code       string
		}{
			{name: "success", path: adID, repository: &fakeRepository{}, status: http.StatusNoContent},
			{name: "invalid ID", path: "not-an-id", repository: &fakeRepository{}, status: http.StatusUnprocessableEntity, code: "VALIDATION_FAILED"},
			{name: "not found", path: adID, repository: &fakeRepository{err: ErrNotFound}, status: http.StatusNotFound, code: "NOT_FOUND"},
			{name: "dependency unavailable", path: adID, repository: &fakeRepository{err: ErrDependencyUnavailable}, status: http.StatusServiceUnavailable, code: "SERVICE_UNAVAILABLE"},
		} {
			t.Run(test.name, func(t *testing.T) {
				router := NewRouter(NewHandler(NewService(test.repository, fakeIDs{}, fakeClock{})), fakeAuth{id: adminID})
				request := httptest.NewRequest(http.MethodDelete, "/api/v1/ads/"+test.path, nil)
				request.Header.Set("Authorization", "Bearer admin-token")
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if response.Code != test.status {
					t.Fatalf("status = %d, want %d", response.Code, test.status)
				}
				if test.status == http.StatusNoContent && response.Body.Len() != 0 {
					t.Fatalf("success response body = %q, want empty", response.Body.String())
				}
				if test.code != "" && !strings.Contains(response.Body.String(), test.code) {
					t.Fatalf("response = %s, want code %s", response.Body.String(), test.code)
				}
			})
		}
	})

	t.Run("router requires an administrator", func(t *testing.T) {
		for _, test := range []struct {
			name      string
			authorize string
			authError error
			status    int
			code      string
		}{
			{name: "missing token", status: http.StatusUnauthorized, code: "UNAUTHORIZED"},
			{name: "invalid token", authorize: "Bearer invalid-token", authError: errors.New("invalid token"), status: http.StatusUnauthorized, code: "INVALID_TOKEN"},
			{name: "regular user", authorize: "Bearer user-token", authError: auth.ErrForbidden, status: http.StatusForbidden, code: "FORBIDDEN"},
		} {
			t.Run(test.name, func(t *testing.T) {
				router := NewRouter(NewHandler(NewService(&fakeRepository{}, fakeIDs{}, fakeClock{})), fakeAuth{err: test.authError})
				request := httptest.NewRequest(http.MethodDelete, "/api/v1/ads/"+adID, nil)
				request.Header.Set("Authorization", test.authorize)
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				if response.Code != test.status || !strings.Contains(response.Body.String(), test.code) {
					t.Fatalf("response = %d %s, want %d containing %s", response.Code, response.Body.String(), test.status, test.code)
				}
			})
		}
	})
}

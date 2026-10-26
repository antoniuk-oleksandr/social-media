package ad

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	"github.com/go-chi/chi/v5"
)

type TokenAuthenticator interface {
	AuthenticateAdmin(context.Context, string) (string, error)
}
type principalKey struct{}

func WithAuthenticatedAdminID(r *http.Request, id string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), principalKey{}, id))
}
func authenticatedAdminID(r *http.Request) (string, bool) {
	id, ok := r.Context().Value(principalKey{}).(string)
	return id, ok && id != ""
}

func NewRouter(handler *Handler, authenticator TokenAuthenticator) http.Handler {
	router := chi.NewRouter()
	router.HandleFunc("POST /api/v1/ads", func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now()
		const prefix = "Bearer "
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, prefix) || strings.TrimSpace(strings.TrimPrefix(header, prefix)) == "" {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", requestID(r))
			handler.logRequest("POST /api/v1/ads", requestID(r), http.StatusUnauthorized, errors.New("UNAUTHORIZED"), time.Since(startedAt))
			return
		}
		id, err := authenticator.AuthenticateAdmin(r.Context(), strings.TrimPrefix(header, prefix))
		if err != nil {
			if errors.Is(err, auth.ErrForbidden) {
				writeError(w, http.StatusForbidden, "FORBIDDEN", "Administrator role required", requestID(r))
				handler.logRequest("POST /api/v1/ads", requestID(r), http.StatusForbidden, errors.New("FORBIDDEN"), time.Since(startedAt))
				return
			}
			writeError(w, http.StatusUnauthorized, "INVALID_TOKEN", "Invalid access token", requestID(r))
			handler.logRequest("POST /api/v1/ads", requestID(r), http.StatusUnauthorized, errors.New("INVALID_TOKEN"), time.Since(startedAt))
			return
		}
		handler.Create(w, WithAuthenticatedAdminID(r, id))
	})
	router.HandleFunc("GET /api/v1/ads/{adId}", func(w http.ResponseWriter, r *http.Request) {
		startedAt := time.Now()
		const prefix = "Bearer "
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, prefix) || strings.TrimSpace(strings.TrimPrefix(header, prefix)) == "" {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", requestID(r))
			handler.logRequest("GET /api/v1/ads/{adId}", requestID(r), http.StatusUnauthorized, errors.New("UNAUTHORIZED"), time.Since(startedAt))
			return
		}
		id, err := authenticator.AuthenticateAdmin(r.Context(), strings.TrimPrefix(header, prefix))
		if err != nil {
			if errors.Is(err, auth.ErrForbidden) {
				writeError(w, http.StatusForbidden, "FORBIDDEN", "Administrator role required", requestID(r))
				handler.logRequest("GET /api/v1/ads/{adId}", requestID(r), http.StatusForbidden, errors.New("FORBIDDEN"), time.Since(startedAt))
				return
			}
			writeError(w, http.StatusUnauthorized, "INVALID_TOKEN", "Invalid access token", requestID(r))
			handler.logRequest("GET /api/v1/ads/{adId}", requestID(r), http.StatusUnauthorized, errors.New("INVALID_TOKEN"), time.Since(startedAt))
			return
		}
		handler.GetByID(w, WithAuthenticatedAdminID(r, id))
	})
	return router
}

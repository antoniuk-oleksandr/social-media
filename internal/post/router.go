package post

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

type TokenAuthenticator interface {
	Authenticate(context.Context, string) (string, error)
}

func NewRouter(handler *Handler, authenticator TokenAuthenticator) http.Handler {
	router := chi.NewRouter()
	router.Get("/api/v1/posts/{postId}", handler.GetByID)
	authenticated := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			startedAt := time.Now()
			const prefix = "Bearer "
			header := r.Header.Get("Authorization")
			if !strings.HasPrefix(header, prefix) || strings.TrimSpace(strings.TrimPrefix(header, prefix)) == "" {
				writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", requestID(r))
				handler.logAuthenticationFailure(requestID(r), "UNAUTHORIZED", http.StatusUnauthorized, time.Since(startedAt))
				return
			}
			id, err := authenticator.Authenticate(r.Context(), strings.TrimPrefix(header, prefix))
			if err != nil {
				writeError(w, http.StatusUnauthorized, "INVALID_TOKEN", "Invalid access token", requestID(r))
				handler.logAuthenticationFailure(requestID(r), "INVALID_TOKEN", http.StatusUnauthorized, time.Since(startedAt))
				return
			}
			next(w, WithAuthenticatedUserID(r, id))
		}
	}
	router.HandleFunc("POST /api/v1/posts", authenticated(handler.Create))
	router.HandleFunc("DELETE /api/v1/posts/{postId}", authenticated(handler.Delete))
	return router
}
func requestID(r *http.Request) string {
	if value := r.Header.Get("X-Request-ID"); value != "" {
		return value
	}
	return "req-local"
}

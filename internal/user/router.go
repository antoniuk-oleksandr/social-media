package user

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

type TokenAuthenticator interface {
	Authenticate(context.Context, string) (string, error)
}

func NewRouter(handler *Handler, authenticators ...TokenAuthenticator) http.Handler {
	router := chi.NewRouter()
	router.Get("/api/v1/users/{userId}", handler.GetByID)
	if len(authenticators) == 0 {
		router.Patch("/api/v1/users/me", handler.UpdateMe)
	} else {
		router.With(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				const prefix = "Bearer "
				header := r.Header.Get("Authorization")
				if !strings.HasPrefix(header, prefix) || strings.TrimSpace(strings.TrimPrefix(header, prefix)) == "" {
					writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", requestID(r))
					return
				}
				id, err := authenticators[0].Authenticate(r.Context(), strings.TrimPrefix(header, prefix))
				if err != nil {
					writeError(w, http.StatusUnauthorized, "INVALID_TOKEN", "Invalid access token", requestID(r))
					return
				}
				next.ServeHTTP(w, WithAuthenticatedUserID(r, id))
			})
		}).Patch("/api/v1/users/me", handler.UpdateMe)
	}
	return router
}

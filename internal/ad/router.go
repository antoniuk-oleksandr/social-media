package ad

import (
	"context"
	"net/http"
	"strings"

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
		const prefix = "Bearer "
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, prefix) || strings.TrimSpace(strings.TrimPrefix(header, prefix)) == "" {
			writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", requestID(r))
			return
		}
		id, err := authenticator.AuthenticateAdmin(r.Context(), strings.TrimPrefix(header, prefix))
		if err != nil {
			if strings.Contains(err.Error(), "forbidden") {
				writeError(w, http.StatusForbidden, "FORBIDDEN", "Administrator role required", requestID(r))
				return
			}
			writeError(w, http.StatusUnauthorized, "INVALID_TOKEN", "Invalid access token", requestID(r))
			return
		}
		handler.Create(w, WithAuthenticatedAdminID(r, id))
	})
	return router
}

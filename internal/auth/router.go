package auth

import (
	"github.com/go-chi/chi/v5"
	"net/http"
)

func Router(h Handler) http.Handler {
	r := chi.NewRouter()
	r.Post("/api/v1/auth/register", h.Register)
	return r
}

package user

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func NewRouter(handler *Handler) http.Handler {
	router := chi.NewRouter()
	router.Get("/api/v1/users/{userId}", handler.GetByID)
	return router
}

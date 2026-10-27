package feed

import (
	"context"
	"net/http"
	"strings"
)

type TokenAuthenticator interface {
	Authenticate(context.Context, string) (string, error)
}

func NewRouter(h *Handler, a TokenAuthenticator) http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("/api/v1/feed", func(w http.ResponseWriter, r *http.Request) {
		if token := r.Header.Get("Authorization"); token != "" {
			const p = "Bearer "
			value := strings.TrimPrefix(token, p)
			if !strings.HasPrefix(token, p) || !isToken68(value) {
				writeError(w, 401, "INVALID_TOKEN", "Invalid access token", requestID(r))
				return
			}
			id, e := a.Authenticate(r.Context(), strings.TrimPrefix(token, p))
			if e != nil {
				writeError(w, 401, "INVALID_TOKEN", "Invalid access token", requestID(r))
				return
			}
			r = WithAuthenticatedUserID(r, id)
		}
		h.Get(w, r)
	})
	return m
}

func isToken68(value string) bool {
	if value == "" {
		return false
	}
	credential := false
	padding := false
	for _, char := range value {
		switch {
		case char == '=':
			padding = true
		case padding:
			return false
		case char >= 'a' && char <= 'z', char >= 'A' && char <= 'Z', char >= '0' && char <= '9':
			credential = true
		case strings.ContainsRune("-._~+/", char):
			credential = true
		default:
			return false
		}
	}
	return credential
}

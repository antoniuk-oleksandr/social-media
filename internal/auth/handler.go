package auth

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/antoniuk-oleksandr/social-media/internal/shared/httpjson"
	"net/http"
)

type ServicePort interface {
	Register(context.Context, RegisterCommand) (RegisterResult, error)
}

type Handler struct{ Service ServicePort }

func (h Handler) Register(w http.ResponseWriter, r *http.Request) {
	rid := r.Header.Get("X-Request-ID")
	if rid == "" {
		rid = "req-local"
	}
	if r.Header.Get("Content-Type") != "application/json" {
		writeErr(w, 415, "INVALID_JSON", "Content-Type must be application/json", rid)
		return
	}
	var req RegisterRequest
	if e := httpjson.Decode(r, &req); errors.Is(e, httpjson.ErrPayloadTooLarge) {
		writeErr(w, 413, "PAYLOAD_TOO_LARGE", "Request body is too large", rid)
		return
	} else if e != nil {
		writeErr(w, 400, "INVALID_JSON", "Malformed JSON", rid)
		return
	}
	res, e := h.Service.Register(r.Context(), RegisterCommand{req.Username, req.Email, req.Password, req.DisplayName})
	if e != nil {
		code, status, msg := "INTERNAL_ERROR", 500, "Internal server error"
		switch {
		case errors.Is(e, ErrValidation):
			code, status, msg = "VALIDATION_FAILED", 422, "Invalid registration fields"
		case errors.Is(e, ErrUsernameTaken):
			code, status, msg = "USERNAME_TAKEN", 409, "Username is already registered"
		case errors.Is(e, ErrEmailTaken):
			code, status, msg = "EMAIL_TAKEN", 409, "Email is already registered"
		case errors.Is(e, ErrDependencyUnavailable):
			code, status, msg = "SERVICE_UNAVAILABLE", http.StatusServiceUnavailable, "Service temporarily unavailable"
		}
		writeErr(w, status, code, msg, rid)
		return
	}
	out := RegisterResponse{User: UserResponse{res.User.ID, res.User.Username, res.User.Email, res.User.DisplayName, string(res.User.Role), res.User.CreatedAt.UTC().Format("2006-01-02T15:04:05Z")}, AccessToken: res.Tokens.AccessToken, RefreshToken: res.Tokens.RefreshToken, TokenType: "Bearer", ExpiresIn: res.Tokens.ExpiresIn, RefreshExpiresIn: res.Tokens.RefreshExpiresIn}
	writeJSON(w, http.StatusCreated, out, rid)
}
func writeErr(w http.ResponseWriter, status int, code, msg, rid string) {
	writeJSON(w, status, ErrorResponse{Error: ErrorBody{Code: code, Message: msg, RequestID: rid}}, rid)
}
func writeJSON(w http.ResponseWriter, status int, value any, rid string) {
	body, err := json.Marshal(value)
	if err != nil {
		body, err = json.Marshal(ErrorResponse{Error: ErrorBody{Code: "INTERNAL_ERROR", Message: "Internal server error", RequestID: rid}})
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		status = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(body); err != nil {
		return
	}
}

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/platform/logging"
	"github.com/antoniuk-oleksandr/social-media/internal/shared/httpjson"
)

type ServicePort interface {
	Register(context.Context, RegisterCommand) (RegisterResult, error)
	Login(context.Context, LoginCommand) (RegisterResult, error)
}

type Handler struct {
	Service ServicePort
	Logger  logging.Logger
}

func (h Handler) Login(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	rid := requestID(r)
	status := http.StatusOK
	logCode := ""
	var logErr error
	loggedWriter := &responseWriter{ResponseWriter: w}
	defer func() {
		if loggedWriter.status != 0 {
			status = loggedWriter.status
		}
		h.logLoginRequest(rid, status, logCode, logErr, time.Since(startedAt))
	}()
	if r.Header.Get("Content-Type") != "application/json" {
		status, logCode = http.StatusUnsupportedMediaType, "INVALID_JSON"
		writeErr(loggedWriter, status, logCode, "Content-Type must be application/json", rid)
		return
	}
	var req LoginRequest
	if err := httpjson.Decode(r, &req); errors.Is(err, httpjson.ErrPayloadTooLarge) {
		status, logCode = http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE"
		writeErr(loggedWriter, status, logCode, "Request body is too large", rid)
		return
	} else if err != nil {
		status, logCode = http.StatusBadRequest, "INVALID_JSON"
		writeErr(loggedWriter, status, logCode, "Malformed JSON", rid)
		return
	}
	res, err := h.Service.Login(r.Context(), LoginCommand{Login: req.Login, Password: req.Password})
	if err != nil {
		if errors.Is(err, ErrValidation) {
			status, logCode = http.StatusUnprocessableEntity, "VALIDATION_FAILED"
			writeErr(loggedWriter, status, logCode, "Invalid login fields", rid)
			return
		}
		if errors.Is(err, ErrInvalidCredentials) {
			status, logCode = http.StatusUnauthorized, "INVALID_CREDENTIALS"
			writeErr(loggedWriter, status, logCode, "Invalid credentials", rid)
			return
		}
		if errors.Is(err, ErrDependencyUnavailable) {
			status, logCode = http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE"
			logErr = err
			writeErr(loggedWriter, status, logCode, "Service temporarily unavailable", rid)
			return
		}
		status, logCode = http.StatusInternalServerError, "INTERNAL_ERROR"
		logErr = err
		writeErr(loggedWriter, status, logCode, "Internal server error", rid)
		return
	}
	out := LoginResponse{User: LoginUserResponse{ID: res.User.ID, Username: res.User.Username, DisplayName: res.User.DisplayName, Role: string(res.User.Role)}, AccessToken: res.Tokens.AccessToken, RefreshToken: res.Tokens.RefreshToken, TokenType: "Bearer", ExpiresIn: res.Tokens.ExpiresIn, RefreshExpiresIn: res.Tokens.RefreshExpiresIn}
	writeJSON(loggedWriter, http.StatusOK, out, rid)
}

type responseWriter struct {
	http.ResponseWriter
	status int
}

func (w *responseWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (h Handler) logLoginRequest(requestID string, status int, code string, err error, duration time.Duration) {
	if h.Logger == nil {
		return
	}
	fields := []logging.Field{
		logging.String("request_id", requestID),
		logging.String("route", "POST /api/v1/auth/login"),
		logging.String("status", strconv.Itoa(status)),
		logging.String("duration", duration.String()),
	}
	if code != "" {
		fields = append(fields, logging.String("error_code", code))
	}
	if err != nil {
		fields = append(fields, logging.Error(err))
		if logErr := h.Logger.Error("login internal failure", fields...); logErr != nil {
			return
		}
		return
	}
	if err := h.Logger.Info("login request completed", fields...); err != nil {
		return
	}
}

func requestID(r *http.Request) string {
	if rid := r.Header.Get("X-Request-ID"); rid != "" {
		return rid
	}
	return "req-local"
}

func (h Handler) Register(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
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
		h.logRegistrationFailure(rid, code, status, e, time.Since(startedAt))
		writeErr(w, status, code, msg, rid)
		return
	}
	out := RegisterResponse{User: UserResponse{res.User.ID, res.User.Username, res.User.Email, res.User.DisplayName, string(res.User.Role), res.User.CreatedAt.UTC().Format("2006-01-02T15:04:05Z")}, AccessToken: res.Tokens.AccessToken, RefreshToken: res.Tokens.RefreshToken, TokenType: "Bearer", ExpiresIn: res.Tokens.ExpiresIn, RefreshExpiresIn: res.Tokens.RefreshExpiresIn}
	writeJSON(w, http.StatusCreated, out, rid)
}

func (h Handler) logRegistrationFailure(requestID, code string, status int, err error, duration time.Duration) {
	if h.Logger == nil {
		return
	}
	if logErr := h.Logger.Error("registration request failed", logging.String("request_id", requestID), logging.String("route", "POST /api/v1/auth/register"), logging.String("error_code", code), logging.String("status", http.StatusText(status)), logging.String("duration", duration.String()), logging.Error(err)); logErr != nil {
		return
	}
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

package user

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/platform/logging"
	platformobservability "github.com/antoniuk-oleksandr/social-media/internal/platform/observability"
	"github.com/antoniuk-oleksandr/social-media/internal/shared/httpjson"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service Service
	logger  logging.Logger
}

func NewHandler(service Service, loggers ...logging.Logger) *Handler {
	var logger logging.Logger
	if len(loggers) > 0 {
		logger = loggers[0]
	}
	return &Handler{service: service, logger: logger}
}

func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	rid := requestID(r)
	loggedWriter := &responseWriter{ResponseWriter: w}
	status := http.StatusOK
	var requestErr error
	var userID *string
	defer func() {
		if loggedWriter.status != 0 {
			status = loggedWriter.status
		}
		h.logRequest(rid, status, requestErr, userID, time.Since(startedAt))
	}()

	id := chi.URLParam(r, "userId")
	if !validUUIDv7(id) {
		requestErr = ErrInvalidID
		writeError(loggedWriter, http.StatusUnprocessableEntity, "VALIDATION_FAILED", "Invalid user ID", rid)
		return
	}
	userID = &id
	value, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		requestErr = err
		status, code, message := http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error"
		switch {
		case errors.Is(err, ErrInvalidID):
			status, code, message = http.StatusUnprocessableEntity, "VALIDATION_FAILED", "Invalid user ID"
		case errors.Is(err, ErrNotFound):
			status, code, message = http.StatusNotFound, "NOT_FOUND", "User not found"
		case errors.Is(err, ErrDependencyUnavailable):
			status, code, message = http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Service temporarily unavailable"
		}
		writeError(loggedWriter, status, code, message, rid)
		return
	}
	writeJSON(loggedWriter, http.StatusOK, ToProfileResponse(value))
}

type principalKey struct{}

func WithAuthenticatedUserID(r *http.Request, id string) *http.Request {
	ctx := context.WithValue(r.Context(), principalKey{}, id)
	return r.WithContext(platformobservability.WithUserID(ctx, id))
}

func authenticatedUserID(r *http.Request) (string, bool) {
	id, ok := r.Context().Value(principalKey{}).(string)
	return id, ok && id != ""
}

func (h *Handler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	rid := requestID(r)
	status := http.StatusOK
	var requestErr error
	var userID *string
	loggedWriter := &responseWriter{ResponseWriter: w}
	defer func() {
		if loggedWriter.status != 0 {
			status = loggedWriter.status
		}
		h.logUpdateRequest(rid, status, requestErr, userID, time.Since(startedAt))
	}()

	authenticatedID, ok := authenticatedUserID(r)
	if !ok {
		writeError(loggedWriter, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", rid)
		return
	}
	userID = &authenticatedID
	if r.Header.Get("Content-Type") != "application/json" {
		writeError(loggedWriter, http.StatusBadRequest, "INVALID_JSON", "Content-Type must be application/json", rid)
		return
	}
	var request UpdateProfileRequest
	if err := httpjson.Decode(r, &request); err != nil {
		status, code, message := http.StatusBadRequest, "INVALID_JSON", "Invalid JSON"
		if errors.Is(err, httpjson.ErrPayloadTooLarge) {
			status, code, message = http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "Request body is too large"
		}
		writeError(loggedWriter, status, code, message, rid)
		return
	}
	value, err := h.service.UpdateProfile(r.Context(), ToUpdateProfileCommand(authenticatedID, request))
	if err != nil {
		requestErr = err
		status, code, message := http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error"
		switch {
		case errors.Is(err, ErrValidation), errors.Is(err, ErrInvalidID):
			status, code, message = http.StatusUnprocessableEntity, "VALIDATION_FAILED", "Invalid profile fields"
		case errors.Is(err, ErrUsernameTaken):
			status, code, message = http.StatusConflict, "USERNAME_TAKEN", "Username is already registered"
		case errors.Is(err, ErrNotFound):
			status, code, message = http.StatusNotFound, "NOT_FOUND", "User not found"
		case errors.Is(err, ErrDependencyUnavailable):
			status, code, message = http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Service temporarily unavailable"
		}
		writeError(loggedWriter, status, code, message, rid)
		return
	}
	writeJSON(loggedWriter, http.StatusOK, ToProfileResponse(value))
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

func (h *Handler) logRequest(requestID string, status int, requestErr error, userID *string, duration time.Duration) {
	if h.logger == nil {
		return
	}
	fields := []logging.Field{
		logging.String("request_id", requestID),
		logging.String("route", "GET /api/v1/users/{userId}"),
		logging.String("status", fmt.Sprint(status)),
		logging.String("duration", duration.String()),
	}
	if userID != nil {
		fields = append(fields, logging.String("user_id", *userID))
	}
	if requestErr != nil {
		fields = append(fields, logging.Error(requestErr))
		if err := h.logger.Error("user request failed", fields...); err != nil {
			return
		}
		return
	}
	if err := h.logger.Info("user request completed", fields...); err != nil {
		return
	}
}

func (h *Handler) logUpdateRequest(requestID string, status int, requestErr error, userID *string, duration time.Duration) {
	if h.logger == nil {
		return
	}
	fields := []logging.Field{
		logging.String("request_id", requestID),
		logging.String("route", "PATCH /api/v1/users/me"),
		logging.String("status", strconv.Itoa(status)),
		logging.String("duration", duration.String()),
	}
	if userID != nil {
		fields = append(fields, logging.String("user_id", *userID))
	}
	if requestErr != nil {
		fields = append(fields, logging.Error(requestErr))
		if err := h.logger.Error("user request failed", fields...); err != nil {
			return
		}
		return
	}
	if err := h.logger.Info("user request completed", fields...); err != nil {
		return
	}
}

func requestID(r *http.Request) string {
	if value := r.Header.Get("X-Request-ID"); value != "" {
		return value
	}
	return "req-local"
}

type errorResponse struct {
	Error errorBody `json:"error"`
}

type errorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestId"`
}

func writeError(w http.ResponseWriter, status int, code, message, requestID string) {
	writeJSON(w, status, errorResponse{Error: errorBody{Code: code, Message: message, RequestID: requestID}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		return
	}
}

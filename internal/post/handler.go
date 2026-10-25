package post

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/platform/logging"
	"github.com/antoniuk-oleksandr/social-media/internal/shared/httpjson"
	"github.com/go-chi/chi/v5"
)

type Handler struct {
	service Service
	logger  logging.Logger
}

func NewHandler(service Service, logger logging.Logger) *Handler {
	return &Handler{service: service, logger: logger}
}

type principalKey struct{}

func WithAuthenticatedUserID(r *http.Request, id string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), principalKey{}, id))
}
func authenticatedUserID(r *http.Request) (string, bool) {
	id, ok := r.Context().Value(principalKey{}).(string)
	return id, ok && id != ""
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	rid := requestID(r)
	status := http.StatusCreated
	var requestErr error
	var authorIDForLog *string
	loggedWriter := &responseWriter{ResponseWriter: w}
	defer func() {
		if loggedWriter.status != 0 {
			status = loggedWriter.status
		}
		h.logCreateRequest(rid, status, requestErr, authorIDForLog, time.Since(startedAt))
	}()

	authorID, ok := authenticatedUserID(r)
	if !ok {
		writeError(loggedWriter, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", rid)
		return
	}
	authorIDForLog = &authorID
	if r.Header.Get("Content-Type") != "application/json" {
		writeError(loggedWriter, http.StatusBadRequest, "INVALID_JSON", "Content-Type must be application/json", rid)
		return
	}
	var request CreateRequest
	if err := httpjson.Decode(r, &request); err != nil {
		requestErr = err
		status, code, message := http.StatusBadRequest, "INVALID_JSON", "Invalid JSON"
		if errors.Is(err, httpjson.ErrPayloadTooLarge) {
			status, code, message = http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "Request body is too large"
		}
		writeError(loggedWriter, status, code, message, rid)
		return
	}
	value, err := h.service.Create(r.Context(), ToCreateCommand(authorID, request))
	if err != nil {
		requestErr = err
		if errors.Is(err, ErrValidation) {
			writeError(loggedWriter, http.StatusUnprocessableEntity, "VALIDATION_FAILED", "Invalid post content", rid)
			return
		}
		if errors.Is(err, ErrDependencyUnavailable) {
			writeError(loggedWriter, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Service temporarily unavailable", rid)
			return
		}
		writeError(loggedWriter, http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error", rid)
		return
	}
	writeJSON(loggedWriter, http.StatusCreated, ToResponse(value))
}

func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	rid := requestID(r)
	loggedWriter := &responseWriter{ResponseWriter: w}
	status := http.StatusOK
	var requestErr error
	var postID *string
	defer func() {
		if loggedWriter.status != 0 {
			status = loggedWriter.status
		}
		h.logGetRequest(rid, status, requestErr, postID, time.Since(startedAt))
	}()

	id := chi.URLParam(r, "postId")
	postID = &id
	value, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		requestErr = err
		status, code, message := http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error"
		switch {
		case errors.Is(err, ErrInvalidID):
			status, code, message = http.StatusUnprocessableEntity, "VALIDATION_FAILED", "Invalid post ID"
		case errors.Is(err, ErrNotFound):
			status, code, message = http.StatusNotFound, "NOT_FOUND", "Post not found"
		case errors.Is(err, ErrDependencyUnavailable):
			status, code, message = http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Service temporarily unavailable"
		}
		writeError(loggedWriter, status, code, message, rid)
		return
	}
	writeJSON(loggedWriter, http.StatusOK, ToResponse(value))
}

func (h *Handler) logGetRequest(requestID string, status int, requestErr error, postID *string, duration time.Duration) {
	if h.logger == nil {
		return
	}
	fields := []logging.Field{
		logging.String("request_id", requestID),
		logging.String("route", "GET /api/v1/posts/{postId}"),
		logging.String("status", strconv.Itoa(status)),
		logging.String("duration", duration.String()),
	}
	if postID != nil {
		fields = append(fields, logging.String("post_id", *postID))
	}
	if requestErr != nil {
		fields = append(fields, logging.Error(requestErr))
		if err := h.logger.Error("post request failed", fields...); err != nil {
			return
		}
		return
	}
	if err := h.logger.Info("post request completed", fields...); err != nil {
		return
	}
}

func (h *Handler) logCreateRequest(requestID string, status int, requestErr error, userID *string, duration time.Duration) {
	if h.logger == nil {
		return
	}
	fields := []logging.Field{
		logging.String("request_id", requestID),
		logging.String("route", "POST /api/v1/posts"),
		logging.String("status", strconv.Itoa(status)),
		logging.String("duration", duration.String()),
	}
	if userID != nil {
		fields = append(fields, logging.String("user_id", *userID))
	}
	if requestErr != nil {
		fields = append(fields, logging.Error(requestErr))
		if err := h.logger.Error("post request failed", fields...); err != nil {
			return
		}
		return
	}
	if err := h.logger.Info("post request completed", fields...); err != nil {
		return
	}
}

func (h *Handler) logAuthenticationFailure(requestID, code string, status int, duration time.Duration) {
	if h.logger == nil {
		return
	}
	fields := []logging.Field{
		logging.String("request_id", requestID),
		logging.String("route", "POST /api/v1/posts"),
		logging.String("status", strconv.Itoa(status)),
		logging.String("duration", duration.String()),
		logging.String("error_code", code),
	}
	if err := h.logger.Info("post request completed", fields...); err != nil {
		return
	}
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

type errorResponse struct {
	Error errorBody `json:"error"`
}
type errorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestId"`
}

func writeError(w http.ResponseWriter, status int, code, message, rid string) {
	writeJSON(w, status, errorResponse{Error: errorBody{Code: code, Message: message, RequestID: rid}})
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		return
	}
}

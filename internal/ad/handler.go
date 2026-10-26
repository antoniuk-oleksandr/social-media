package ad

import (
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

func NewHandler(service Service, loggers ...logging.Logger) *Handler {
	var logger logging.Logger
	if len(loggers) > 0 {
		logger = loggers[0]
	}
	return &Handler{service: service, logger: logger}
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	rid := requestID(r)
	status := http.StatusCreated
	var requestErr error
	loggedWriter := &responseWriter{ResponseWriter: w}
	defer func() {
		if loggedWriter.status != 0 {
			status = loggedWriter.status
		}
		h.logRequest("POST /api/v1/ads", rid, status, requestErr, time.Since(startedAt), adminIDFromRequest(r))
	}()

	adminID, ok := authenticatedAdminID(r)
	if !ok {
		writeError(loggedWriter, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required", rid)
		return
	}
	if r.Header.Get("Content-Type") != "application/json" {
		writeError(loggedWriter, http.StatusBadRequest, "INVALID_JSON", "Content-Type must be application/json", rid)
		return
	}
	var request CreateRequest
	if err := httpjson.Decode(r, &request); err != nil {
		requestErr = err
		status, code, msg := http.StatusBadRequest, "INVALID_JSON", "Invalid JSON"
		if errors.Is(err, httpjson.ErrPayloadTooLarge) {
			status, code, msg = http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "Request body is too large"
		}
		writeError(loggedWriter, status, code, msg, rid)
		return
	}
	command, err := ToCreateCommand(adminID, request)
	if err != nil {
		requestErr = err
		writeError(loggedWriter, http.StatusUnprocessableEntity, "VALIDATION_FAILED", "Invalid advertisement fields", rid)
		return
	}
	value, err := h.service.Create(r.Context(), command)
	if err != nil {
		requestErr = err
		status, code, msg := http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error"
		if errors.Is(err, ErrValidation) {
			status, code, msg = http.StatusUnprocessableEntity, "VALIDATION_FAILED", "Invalid advertisement fields"
		}
		if errors.Is(err, ErrDependencyUnavailable) {
			status, code, msg = http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Service temporarily unavailable"
		}
		writeError(loggedWriter, status, code, msg, rid)
		return
	}
	writeJSON(loggedWriter, http.StatusCreated, ToResponse(value))
}

func (h *Handler) GetByID(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	rid := requestID(r)
	status := http.StatusOK
	var requestErr error
	loggedWriter := &responseWriter{ResponseWriter: w}
	defer func() {
		if loggedWriter.status != 0 {
			status = loggedWriter.status
		}
		h.logRequest("GET /api/v1/ads/{adId}", rid, status, requestErr, time.Since(startedAt), adminIDFromRequest(r))
	}()

	value, err := h.service.GetByID(r.Context(), chi.URLParam(r, "adId"))
	if err != nil {
		requestErr = err
		status, code, message := http.StatusInternalServerError, "INTERNAL_ERROR", "Internal server error"
		switch {
		case errors.Is(err, ErrInvalidID):
			status, code, message = http.StatusUnprocessableEntity, "VALIDATION_FAILED", "Invalid advertisement ID"
		case errors.Is(err, ErrNotFound):
			status, code, message = http.StatusNotFound, "NOT_FOUND", "Advertisement not found"
		case errors.Is(err, ErrDependencyUnavailable):
			status, code, message = http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "Service temporarily unavailable"
		}
		writeError(loggedWriter, status, code, message, rid)
		return
	}
	writeJSON(loggedWriter, http.StatusOK, ToResponse(value))
}

func (h *Handler) logRequest(route, requestID string, status int, requestErr error, duration time.Duration, adminID ...string) {
	if h.logger == nil {
		return
	}
	fields := []logging.Field{
		logging.String("request_id", requestID),
		logging.String("route", route),
		logging.String("status", strconv.Itoa(status)),
		logging.String("duration", duration.String()),
	}
	if len(adminID) > 0 && adminID[0] != "" {
		fields = append(fields, logging.String("admin_id", adminID[0]))
	}
	if requestErr != nil {
		fields = append(fields, logging.Error(requestErr))
		if err := h.logger.Error("ad request failed", fields...); err != nil {
			return
		}
		return
	}
	if err := h.logger.Info("ad request completed", fields...); err != nil {
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

func requestID(r *http.Request) string {
	if id := r.Header.Get("X-Request-ID"); id != "" {
		return id
	}
	return "req-local"
}

func adminIDFromRequest(r *http.Request) string {
	id, _ := authenticatedAdminID(r)
	return id
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

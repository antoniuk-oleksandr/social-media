package feed

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/platform/logging"
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
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	response := &statusWriter{ResponseWriter: w}
	var dependencyFailure bool
	var postCount, adCount int
	var hasMore bool
	defer func() {
		status := response.status
		if status == 0 {
			status = http.StatusOK
		}
		fields := []logging.Field{
			logging.String("request_id", requestID(r)),
			logging.String("route", "GET /api/v1/feed"),
			logging.String("status", strconv.Itoa(status)),
			logging.String("duration", time.Since(started).String()),
		}
		if id, ok := r.Context().Value(principalKey{}).(string); ok && id != "" {
			fields = append(fields, logging.String("user_id", id))
		}
		fields = append(fields,
			logging.String("post_count", strconv.Itoa(postCount)),
			logging.String("ad_count", strconv.Itoa(adCount)),
			logging.String("has_more", strconv.FormatBool(hasMore)),
			logging.String("dependency_failure", strconv.FormatBool(dependencyFailure)),
		)
		if status >= http.StatusBadRequest {
			if err := h.loggerError("feed request failed", fields...); err != nil {
				return
			}
			return
		}
		if err := h.loggerInfo("feed request completed", fields...); err != nil {
			return
		}
	}()
	w = response

	q := Query{UserID: ""}
	values, err := parseQuery(r.URL.RawQuery)
	if err != nil {
		writeError(w, 400, "INVALID_QUERY", "Invalid query", requestID(r))
		return
	}
	if value, present := values["limit"]; present {
		n, e := strconv.Atoi(value)
		if e != nil || value == "" || strconv.Itoa(n) != value {
			writeError(w, 400, "INVALID_QUERY", "Invalid query", requestID(r))
			return
		}
		q.Limit = n
		q.LimitSet = true
	}
	q.Cursor = values["cursor"]
	if strings.TrimSpace(q.Cursor) != q.Cursor {
		writeError(w, 400, "INVALID_CURSOR", "Invalid cursor", requestID(r))
		return
	}
	if id, ok := r.Context().Value(principalKey{}).(string); ok {
		q.UserID = id
	}
	result, e := h.service.Build(r.Context(), q)
	if e != nil {
		code, msg := "INTERNAL_ERROR", "Internal server error"
		status := 500
		switch {
		case errors.Is(e, ErrInvalidQuery):
			status = 400
			code = "INVALID_QUERY"
			msg = "Invalid query"
		case errors.Is(e, ErrInvalidCursor):
			status = 400
			code = "INVALID_CURSOR"
			msg = "Invalid cursor"
		case errors.Is(e, ErrCursorExpired):
			status = 400
			code = "CURSOR_EXPIRED"
			msg = "Cursor expired"
		case errors.Is(e, ErrDependencyUnavailable):
			dependencyFailure = true
			status = 503
			code = "SERVICE_UNAVAILABLE"
			msg = "Service temporarily unavailable"
		}
		writeError(w, status, code, msg, requestID(r))
		return
	}
	for _, item := range result.Items {
		if item.Post != nil {
			postCount++
		}
		if item.Ad != nil {
			adCount++
		}
	}
	hasMore = result.HasMore
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	if e := json.NewEncoder(w).Encode(toResponse(result)); e != nil {
		return
	}
}

func parseQuery(raw string) (map[string]string, error) {
	values := map[string]string{}
	if raw == "" {
		return values, nil
	}
	for _, pair := range strings.Split(raw, "&") {
		if pair == "" {
			return nil, ErrInvalidQuery
		}
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) != 2 {
			return nil, ErrInvalidQuery
		}
		key, err := url.QueryUnescape(parts[0])
		if err != nil || key != "limit" && key != "cursor" || parts[1] == "" {
			return nil, ErrInvalidQuery
		}
		if _, exists := values[key]; exists {
			return nil, ErrInvalidQuery
		}
		value, err := url.QueryUnescape(parts[1])
		if err != nil || value == "" {
			return nil, ErrInvalidQuery
		}
		if key == "limit" {
			n, err := strconv.Atoi(value)
			if err != nil || strconv.Itoa(n) != value {
				return nil, ErrInvalidQuery
			}
		}
		values[key] = value
	}
	return values, nil
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (h *Handler) loggerInfo(message string, fields ...logging.Field) error {
	if h.logger == nil {
		return nil
	}
	return h.logger.Info(message, fields...)
}

func (h *Handler) loggerError(message string, fields ...logging.Field) error {
	if h.logger == nil {
		return nil
	}
	return h.logger.Error(message, fields...)
}

type principalKey struct{}

func WithAuthenticatedUserID(r *http.Request, id string) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), principalKey{}, id))
}
func requestID(r *http.Request) string {
	if v := r.Header.Get("X-Request-ID"); v != "" {
		return v
	}
	return "req-local"
}

type errBody struct {
	Error struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"requestId"`
	} `json:"error"`
}

func writeError(w http.ResponseWriter, status int, code, msg, rid string) {
	var b errBody
	b.Error.Code = code
	b.Error.Message = msg
	b.Error.RequestID = rid
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if e := json.NewEncoder(w).Encode(b); e != nil {
		return
	}
}

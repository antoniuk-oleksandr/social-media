package auth_test

import (
	"net/http/httptest"

	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	"github.com/antoniuk-oleksandr/social-media/internal/platform/logging"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type refreshCoverageLogger struct {
	message string
	fields  []logging.Field
}

func (l *refreshCoverageLogger) Info(message string, fields ...logging.Field) error {
	l.message, l.fields = message, fields
	return nil
}
func (l *refreshCoverageLogger) Error(message string, fields ...logging.Field) error {
	l.message, l.fields = message, fields
	return nil
}
func (l *refreshCoverageLogger) Sync() error { return nil }

var _ = Describe("Refresh handler logging", func() {
	It("logs successful refresh requests", func() {
		logger := &refreshCoverageLogger{}
		service := &refreshHandlerService{result: auth.TokenPair{AccessToken: "access", RefreshToken: "refresh", ExpiresIn: 60, RefreshExpiresIn: 120}}
		h := auth.Handler{Service: service, Logger: logger}
		r := refreshRequest(`{"refreshToken":"old"}`, "application/json")
		r.Header.Set("X-Request-ID", "refresh-request")
		w := httptest.NewRecorder()
		h.Refresh(w, r)
		Expect(w.Code).To(Equal(200))
		Expect(w.Body.String()).To(ContainSubstring(`"accessToken":"access"`))
		Expect(logger.message).To(Equal("refresh request completed"))
	})
})

package auth_test

import (
	"net/http/httptest"

	"github.com/antoniuk-oleksandr/social-media/internal/auth"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
)

var _ = Describe("Refresh handler logging", func() {
	It("logs successful refresh requests", func() {
		logger, loggerState := newLogger(gomock.NewController(GinkgoT()))
		service := newRefreshHandlerService(auth.TokenPair{AccessToken: "access", RefreshToken: "refresh", ExpiresIn: 60, RefreshExpiresIn: 120}, nil)
		h := auth.Handler{Service: service, Logger: logger}
		r := refreshRequest(`{"refreshToken":"old"}`, "application/json")
		r.Header.Set("X-Request-ID", "refresh-request")
		w := httptest.NewRecorder()
		h.Refresh(w, r)
		Expect(w.Code).To(Equal(200))
		Expect(w.Body.String()).To(ContainSubstring(`"accessToken":"access"`))
		Expect(loggerState.message).To(Equal("refresh request completed"))
	})
})

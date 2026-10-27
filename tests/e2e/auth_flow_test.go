//go:build e2e

package e2e

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("authentication flow", func() {
	It("registers, logs in, refreshes, and logs out", func() {
		account := registerE2EUser()
		login := loginE2E(account.Login, account.Password)

		refreshed := authResponse{}
		response := request(http.MethodPost, "/api/v1/auth/refresh", map[string]string{
			"refreshToken": login.RefreshToken,
		}, "", &refreshed)
		Expect(response.StatusCode()).To(Equal(http.StatusOK), response.String())
		Expect(refreshed.AccessToken).NotTo(BeEmpty())

		response = request(http.MethodPost, "/api/v1/auth/logout", map[string]string{
			"refreshToken": refreshed.RefreshToken,
		}, "", nil)
		Expect(response.StatusCode()).To(Equal(http.StatusNoContent), response.String())
	})
})

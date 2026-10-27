//go:build e2e

package e2e

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("user profile flow", func() {
	It("reads and updates the authenticated user's profile", func() {
		account := registerE2EUser()
		profile := map[string]any{}
		response := request(http.MethodGet, "/api/v1/users/"+account.User.ID, nil, account.AccessToken, &profile)
		Expect(response.StatusCode()).To(Equal(http.StatusOK), response.String())
		Expect(profile["id"]).To(Equal(account.User.ID))

		response = request(http.MethodPatch, "/api/v1/users/me", map[string]string{
			"displayName": "Updated E2E User",
		}, account.AccessToken, nil)
		Expect(response.StatusCode()).To(Equal(http.StatusOK), response.String())
	})
})

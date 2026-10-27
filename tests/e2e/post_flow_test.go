//go:build e2e

package e2e

import (
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("post feed flow", func() {
	It("registers, creates a post, and retrieves the feed", func() {
		account := registerE2EUser()
		created := postResponse{}
		response := request(http.MethodPost, "/api/v1/posts", map[string]string{
			"content": "E2E post flow content",
		}, account.AccessToken, &created)
		Expect(response.StatusCode()).To(Equal(http.StatusCreated), response.String())
		Expect(created.ID).NotTo(BeEmpty())

		response = request(http.MethodGet, "/api/v1/feed", nil, account.AccessToken, nil)
		Expect(response.StatusCode()).To(Equal(http.StatusOK), response.String())
	})
})

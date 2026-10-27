//go:build e2e

package e2e

import (
	"net/http"
	"time"

	"github.com/antoniuk-oleksandr/social-media/tests/fixtures"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
)

type feedItem struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
}

type feedResponse struct {
	Items []feedItem `json:"items"`
}

var _ = Describe("mixed feed flow", func() {
	It("returns both a post and an advertisement", func() {
		account := registerE2EUser()
		admin := loginE2EAdmin()

		createdPost := postResponse{}
		response := request(http.MethodPost, "/api/v1/posts", map[string]string{
			"content": "E2E mixed feed post",
		}, account.AccessToken, &createdPost)
		Expect(response.StatusCode()).To(Equal(http.StatusCreated), response.String())

		createdAd := adResponse{}
		adRequest := fixtures.ActiveAdvertisement("E2E feed advertisement", time.Now())
		adRequest.TargetURL = "https://example.com/feed"
		response = request(http.MethodPost, "/api/v1/ads", adRequest, admin.AccessToken, &createdAd)
		Expect(response.StatusCode()).To(Equal(http.StatusCreated), response.String())

		result := feedResponse{}
		response = request(http.MethodGet, "/api/v1/feed", nil, account.AccessToken, &result)
		Expect(response.StatusCode()).To(Equal(http.StatusOK), response.String())
		Expect(result.Items).To(ContainElement(MatchFields(IgnoreExtras, Fields{"Type": Equal("post")})))
		Expect(result.Items).To(ContainElement(MatchFields(IgnoreExtras, Fields{"Type": Equal("ad")})))
	})
})

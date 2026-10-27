//go:build e2e

package e2e

import (
	"net/http"
	"time"

	"github.com/antoniuk-oleksandr/social-media/tests/fixtures"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type adResponse struct {
	ID string `json:"id"`
}

type adsListResponse struct {
	Items []adResponse `json:"items"`
}

var _ = Describe("administrator advertisement flow", func() {
	It("creates, lists, reads, updates, and deletes an advertisement", func() {
		admin := loginE2EAdmin()
		adRequest := fixtures.ActiveAdvertisement("E2E advertisement", time.Now())

		created := adResponse{}
		response := request(http.MethodPost, "/api/v1/ads", adRequest, admin.AccessToken, &created)
		Expect(response.StatusCode()).To(Equal(http.StatusCreated), response.String())
		Expect(created.ID).NotTo(BeEmpty())

		listed := adsListResponse{}
		response = request(http.MethodGet, "/api/v1/ads", nil, admin.AccessToken, &listed)
		Expect(response.StatusCode()).To(Equal(http.StatusOK), response.String())

		response = request(http.MethodGet, "/api/v1/ads/"+created.ID, nil, admin.AccessToken, nil)
		Expect(response.StatusCode()).To(Equal(http.StatusOK), response.String())

		response = request(http.MethodPatch, "/api/v1/ads/"+created.ID, map[string]string{
			"content": "Updated E2E advertisement",
		}, admin.AccessToken, nil)
		Expect(response.StatusCode()).To(Equal(http.StatusOK), response.String())

		response = request(http.MethodDelete, "/api/v1/ads/"+created.ID, nil, admin.AccessToken, nil)
		Expect(response.StatusCode()).To(Equal(http.StatusNoContent), response.String())
	})
})

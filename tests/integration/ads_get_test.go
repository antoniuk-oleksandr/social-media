//go:build integration

package integration

import (
	"context"
	"time"

	adfeature "github.com/antoniuk-oleksandr/social-media/internal/ad"
	addb "github.com/antoniuk-oleksandr/social-media/internal/ad/dynamodb"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("GET advertisement persistence", func() {
	It("round-trips a complete advertisement and maps an absent item", func() {
		now := time.Date(2026, 10, 18, 12, 0, 0, 123000000, time.UTC)
		adID := "0192f8a0-7b00-7000-8000-000000000811"
		adminID := "0192f8a0-7b00-7000-8000-000000000812"
		repository := &addb.Repository{Client: userIntegrationSuite.Client, TableName: adsTable}
		service := adfeature.NewService(repository, integrationIDGenerator{id: adID}, integrationClock{value: now})

		_, err := service.Create(context.Background(), adfeature.CreateCommand{
			AdminID: adminID, Advertiser: "Example Company", Content: "Try our product", TargetURL: "https://example.com",
			StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour), Status: adfeature.StatusActive,
		})
		Expect(err).NotTo(HaveOccurred())

		read, err := service.GetByID(context.Background(), adID)
		Expect(err).NotTo(HaveOccurred())
		Expect(read.ID).To(Equal(adID))
		Expect(read.Advertiser).To(Equal("Example Company"))
		Expect(read.Content).To(Equal("Try our product"))
		Expect(read.TargetURL).To(Equal("https://example.com"))
		Expect(read.Status).To(Equal(adfeature.StatusActive))
		Expect(read.CreatedAt).To(Equal(now))
		Expect(read.CreatedAtEpoch).To(Equal(now.UnixMilli()))

		_, err = service.GetByID(context.Background(), "0192f8a0-7b00-7000-8000-000000000899")
		Expect(err).To(MatchError(adfeature.ErrNotFound))
	})
})

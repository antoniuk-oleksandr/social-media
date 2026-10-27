//go:build integration

package integration

import (
	"context"
	"fmt"
	"time"

	adfeature "github.com/antoniuk-oleksandr/social-media/internal/ad"
	addb "github.com/antoniuk-oleksandr/social-media/internal/ad/dynamodb"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("PATCH advertisement persistence", func() {
	It("round-trips partial updates and preserves immutable advertisement fields", func() {
		now := time.Date(2026, 10, 18, 12, 0, 0, 123000000, time.UTC)
		adID := "0192f8a0-7b00-7000-8000-000000000821"
		adminID := "0192f8a0-7b00-7000-8000-000000000822"
		repository := &addb.Repository{Client: userIntegrationSuite.Client, TableName: adsTable}
		service := adfeature.NewService(repository, integrationIDGenerator{id: adID}, integrationClock{value: now})

		created, err := service.Create(context.Background(), adfeature.CreateCommand{
			AdminID: adminID, Advertiser: "Example Company", Content: "Original copy", TargetURL: "https://example.com/original",
			StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour), Status: adfeature.StatusActive,
		})
		Expect(err).NotTo(HaveOccurred())

		updated, err := service.Update(context.Background(), adfeature.UpdateCommand{
			AdID:    adID,
			Content: statusString("Updated copy"),
			Status:  adStatusPointer(adfeature.StatusPaused),
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(updated.ID).To(Equal(created.ID))
		Expect(updated.Content).To(Equal("Updated copy"))
		Expect(updated.Status).To(Equal(adfeature.StatusPaused))
		Expect(updated.Advertiser).To(Equal(created.Advertiser))
		Expect(updated.TargetURL).To(Equal(created.TargetURL))
		Expect(updated.StartsAt).To(Equal(created.StartsAt))
		Expect(updated.EndsAt).To(Equal(created.EndsAt))
		Expect(updated.CreatedAt).To(Equal(created.CreatedAt))
		Expect(updated.CreatedBy).To(Equal(created.CreatedBy))

		stored, err := userIntegrationSuite.Client.GetItem(context.Background(), &dynamodb.GetItemInput{
			TableName: aws.String(adsTable),
			Key:       map[string]types.AttributeValue{"adId": &types.AttributeValueMemberS{Value: adID}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(stored.Item["content"]).To(Equal(&types.AttributeValueMemberS{Value: "Updated copy"}))
		Expect(stored.Item["status"]).To(Equal(&types.AttributeValueMemberS{Value: "paused"}))
		Expect(stored.Item["createdBy"]).To(Equal(&types.AttributeValueMemberS{Value: adminID}))
		Expect(stored.Item["createdAtEpoch"]).To(Equal(&types.AttributeValueMemberN{Value: fmt.Sprint(created.CreatedAtEpoch)}))
	})

	It("rejects a repository update when the target item does not exist", func() {
		repository := &addb.Repository{Client: userIntegrationSuite.Client, TableName: adsTable}
		missing := adfeature.Advertisement{
			ID: "0192f8a0-7b00-7000-8000-000000000823", Type: "ad", Advertiser: "Example", Content: "Missing",
			TargetURL: "https://example.com", StartsAt: time.Date(2026, 10, 18, 11, 0, 0, 0, time.UTC),
			EndsAt: time.Date(2026, 10, 18, 13, 0, 0, 0, time.UTC), Status: adfeature.StatusDraft,
		}

		Expect(repository.Update(context.Background(), missing)).To(MatchError(adfeature.ErrNotFound))
	})
})

func statusString(value string) *string { return &value }

func adStatusPointer(value adfeature.Status) *adfeature.Status { return &value }

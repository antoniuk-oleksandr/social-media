//go:build integration

package userintegration

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

var _ = Describe("DELETE advertisement persistence", func() {
	It("removes an existing ad and makes it unavailable by ID", func() {
		now := time.Date(2026, 10, 18, 12, 0, 0, 123000000, time.UTC)
		adID := "0192f8a0-7b00-7000-8000-000000000831"
		repository := &addb.Repository{Client: userIntegrationSuite.Client, TableName: adsTable}
		service := adfeature.NewService(repository, integrationIDGenerator{id: adID}, integrationClock{value: now})

		_, err := service.Create(context.Background(), adfeature.CreateCommand{
			AdminID: "0192f8a0-7b00-7000-8000-000000000832", Advertiser: "Example", Content: "Delete me", TargetURL: "https://example.com",
			StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour), Status: adfeature.StatusActive,
		})
		Expect(err).NotTo(HaveOccurred())

		Expect(service.Delete(context.Background(), adID)).To(Succeed())
		_, err = service.GetByID(context.Background(), adID)
		Expect(err).To(MatchError(adfeature.ErrNotFound))
		Expect(service.Delete(context.Background(), adID)).To(MatchError(adfeature.ErrNotFound))
	})

	It("excludes a deleted ad at the persistence boundary used for feed eligibility", func() {
		now := time.Date(2026, 10, 18, 12, 0, 0, 123000000, time.UTC)
		adID := "0192f8a0-7b00-7000-8000-000000000834"
		repository := &addb.Repository{Client: userIntegrationSuite.Client, TableName: adsTable}
		service := adfeature.NewService(repository, integrationIDGenerator{id: adID}, integrationClock{value: now})

		_, err := service.Create(context.Background(), adfeature.CreateCommand{
			AdminID: "0192f8a0-7b00-7000-8000-000000000835", Advertiser: "Example", Content: "Feed exclusion", TargetURL: "https://example.com",
			StartsAt: now.Add(-time.Hour), EndsAt: now.Add(time.Hour), Status: adfeature.StatusActive,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(service.Delete(context.Background(), adID)).To(Succeed())

		// The feed package is not present yet. Scan the ads persistence boundary
		// with the active-status eligibility predicate and prove deletion removes
		// the ad from the candidate set before feed selection.
		eligible, err := userIntegrationSuite.Client.Scan(context.Background(), &dynamodb.ScanInput{
			TableName:                 aws.String(adsTable),
			FilterExpression:          aws.String("#status = :active"),
			ExpressionAttributeNames:  map[string]string{"#status": "status"},
			ExpressionAttributeValues: map[string]types.AttributeValue{":active": &types.AttributeValueMemberS{Value: string(adfeature.StatusActive)}},
		})
		Expect(err).NotTo(HaveOccurred())
		for _, item := range eligible.Items {
			Expect(item["adId"]).NotTo(Equal(&types.AttributeValueMemberS{Value: adID}), fmt.Sprintf("deleted ad %s remained feed-eligible", adID))
		}
	})

	It("does not delete a missing ad", func() {
		repository := &addb.Repository{Client: userIntegrationSuite.Client, TableName: adsTable}
		Expect(repository.Delete(context.Background(), "0192f8a0-7b00-7000-8000-000000000833")).To(MatchError(adfeature.ErrNotFound))
	})
})

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

var adsTable string

var _ = BeforeEach(func() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	adsTable = fmt.Sprintf("tests-ads-%d", time.Now().UTC().UnixNano())
	_, err := userIntegrationSuite.Client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName:            aws.String(adsTable),
		AttributeDefinitions: []types.AttributeDefinition{{AttributeName: aws.String("adId"), AttributeType: types.ScalarAttributeTypeS}},
		KeySchema:            []types.KeySchemaElement{{AttributeName: aws.String("adId"), KeyType: types.KeyTypeHash}},
		BillingMode:          types.BillingModePayPerRequest,
	})
	Expect(err).NotTo(HaveOccurred())
})

var _ = Describe("POST ad persistence", func() {
	It("stores the complete advertisement in an isolated DynamoDB table", func() {
		now := time.Date(2026, 10, 18, 12, 0, 0, 123000000, time.UTC)
		adID := "0192f8a0-7b00-7000-8000-000000000801"
		adminID := "0192f8a0-7b00-7000-8000-000000000802"
		startsAt := now.Add(-time.Hour)
		endsAt := now.Add(time.Hour)
		repository := &addb.Repository{Client: userIntegrationSuite.Client, TableName: adsTable}
		service := adfeature.NewService(repository, integrationIDGenerator{id: adID}, integrationClock{value: now})

		created, err := service.Create(context.Background(), adfeature.CreateCommand{
			AdminID: adminID, Advertiser: " Example Company ", Content: " Try our product ", TargetURL: "https://example.com", StartsAt: startsAt, EndsAt: endsAt, Status: adfeature.StatusActive,
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(created.ID).To(Equal(adID))
		Expect(created.Advertiser).To(Equal("Example Company"))
		Expect(created.Content).To(Equal("Try our product"))
		Expect(created.CreatedAtEpoch).To(Equal(now.UnixMilli()))

		stored, err := userIntegrationSuite.Client.GetItem(context.Background(), &dynamodb.GetItemInput{
			TableName: aws.String(adsTable),
			Key:       map[string]types.AttributeValue{"adId": &types.AttributeValueMemberS{Value: adID}},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(stored.Item["adId"]).To(Equal(&types.AttributeValueMemberS{Value: adID}))
		Expect(stored.Item["entityType"]).To(Equal(&types.AttributeValueMemberS{Value: "ad"}))
		Expect(stored.Item["advertiser"]).To(Equal(&types.AttributeValueMemberS{Value: "Example Company"}))
		Expect(stored.Item["content"]).To(Equal(&types.AttributeValueMemberS{Value: "Try our product"}))
		Expect(stored.Item["targetUrl"]).To(Equal(&types.AttributeValueMemberS{Value: "https://example.com"}))
		Expect(stored.Item["status"]).To(Equal(&types.AttributeValueMemberS{Value: "active"}))
		Expect(stored.Item["createdBy"]).To(Equal(&types.AttributeValueMemberS{Value: adminID}))
		Expect(stored.Item["createdAtEpoch"]).To(Equal(&types.AttributeValueMemberN{Value: fmt.Sprint(now.UnixMilli())}))
	})
})

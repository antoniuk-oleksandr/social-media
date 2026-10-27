//go:build integration

package integration

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	adfeature "github.com/antoniuk-oleksandr/social-media/internal/ad"
	addb "github.com/antoniuk-oleksandr/social-media/internal/ad/dynamodb"
	platformconfig "github.com/antoniuk-oleksandr/social-media/internal/platform/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var adsListTable string

func setupAdsListTable() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	adsListTable = fmt.Sprintf("tests-ads-list-%d", time.Now().UTC().UnixNano())
	_, err := userIntegrationSuite.Client.CreateTable(ctx, &dynamodb.CreateTableInput{
		TableName: aws.String(adsListTable),
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("adId"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("status"), AttributeType: types.ScalarAttributeTypeS},
			{AttributeName: aws.String("createdAtEpoch"), AttributeType: types.ScalarAttributeTypeN},
		},
		KeySchema: []types.KeySchemaElement{{AttributeName: aws.String("adId"), KeyType: types.KeyTypeHash}},
		GlobalSecondaryIndexes: []types.GlobalSecondaryIndex{{
			IndexName: aws.String("ActiveAdsIndex"),
			KeySchema: []types.KeySchemaElement{
				{AttributeName: aws.String("status"), KeyType: types.KeyTypeHash},
				{AttributeName: aws.String("createdAtEpoch"), KeyType: types.KeyTypeRange},
			},
			Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
		}},
		BillingMode: types.BillingModePayPerRequest,
	})
	Expect(err).NotTo(HaveOccurred())
}

var _ = AfterEach(func() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	output, err := userIntegrationSuite.Client.Scan(ctx, &dynamodb.ScanInput{TableName: aws.String(adsListTable), ProjectionExpression: aws.String("adId")})
	Expect(err).NotTo(HaveOccurred())
	for _, item := range output.Items {
		id, ok := item["adId"].(*types.AttributeValueMemberS)
		Expect(ok).To(BeTrue())
		_, err = userIntegrationSuite.Client.DeleteItem(ctx, &dynamodb.DeleteItemInput{TableName: aws.String(adsListTable), Key: map[string]types.AttributeValue{"adId": id}})
		Expect(err).NotTo(HaveOccurred())
	}
})

func putListAdvertisement(table, id, status string, epoch int64) error {
	return putListAdvertisementWithContent(table, id, status, epoch, "content")
}

func putListAdvertisementWithContent(table, id, status string, epoch int64, content string) error {
	created := time.UnixMilli(epoch).UTC().Format(time.RFC3339Nano)
	_, err := userIntegrationSuite.Client.PutItem(context.Background(), &dynamodb.PutItemInput{
		TableName: aws.String(table),
		Item: map[string]types.AttributeValue{
			"adId": &types.AttributeValueMemberS{Value: id}, "entityType": &types.AttributeValueMemberS{Value: "ad"},
			"advertiser": &types.AttributeValueMemberS{Value: "Example"}, "content": &types.AttributeValueMemberS{Value: content},
			"targetUrl": &types.AttributeValueMemberS{Value: "https://example.com"}, "startsAt": &types.AttributeValueMemberS{Value: created},
			"endsAt": &types.AttributeValueMemberS{Value: created}, "status": &types.AttributeValueMemberS{Value: status},
			"createdBy": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000901"},
			"createdAt": &types.AttributeValueMemberS{Value: created}, "createdAtEpoch": &types.AttributeValueMemberN{Value: strconv.FormatInt(epoch, 10)},
		},
	})
	return err
}

var _ = Describe("GET advertisement list DynamoDB persistence", func() {
	It("filters by status, orders newest first, and continues without duplicates", func() {
		ids := []string{
			"0192f8a0-7b00-7000-0000-000000000901",
			"0192f8a0-7b00-7000-0000-000000000902",
			"0192f8a0-7b00-7000-0000-000000000903",
		}
		for index, id := range ids {
			Expect(putListAdvertisement(adsListTable, id, string(adfeature.StatusActive), int64(1000+index))).To(Succeed())
		}
		Expect(putListAdvertisement(adsListTable, "0192f8a0-7b00-7000-0000-000000000904", string(adfeature.StatusPaused), 2000)).To(Succeed())

		repository := addb.NewConfiguredRepository(userIntegrationSuite.Client, platformconfig.AdsConfig{AdsTable: adsListTable, CursorKey: "integration-test-cursor-key", CursorKeyID: "test", CursorVersion: 1})
		first, err := repository.List(context.Background(), adfeature.ListQuery{Status: adfeature.StatusActive, Limit: 2})
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Items).To(HaveLen(2))
		Expect(first.Items[0].ID).To(Equal(ids[2]))
		Expect(first.Items[1].ID).To(Equal(ids[1]))
		Expect(first.HasMore).To(BeTrue())

		second, err := repository.List(context.Background(), adfeature.ListQuery{Status: adfeature.StatusActive, Limit: 2, Cursor: first.NextCursor})
		Expect(err).NotTo(HaveOccurred())
		Expect(second.Items).To(HaveLen(1))
		Expect(second.Items[0].ID).To(Equal(ids[0]))
		Expect(second.Items[0].Status).To(Equal(adfeature.StatusActive))
		Expect(second.HasMore).To(BeFalse())
		Expect(second.NextCursor).To(BeEmpty())
	})
})

var _ = Describe("GET advertisement list query isolation", func() {
	It("does not return paused records when querying active status", func() {
		activeID := "0192f8a0-7b00-7000-0000-000000000911"
		pausedID := "0192f8a0-7b00-7000-0000-000000000912"
		Expect(putListAdvertisement(adsListTable, activeID, string(adfeature.StatusActive), 3000)).To(Succeed())
		Expect(putListAdvertisement(adsListTable, pausedID, string(adfeature.StatusPaused), 4000)).To(Succeed())

		result, err := (&addb.Repository{Client: userIntegrationSuite.Client, TableName: adsListTable}).List(context.Background(), adfeature.ListQuery{Status: adfeature.StatusActive, Limit: 10})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.Items).To(HaveLen(1))
		Expect(result.Items[0].ID).To(Equal(activeID))
	})
})

var _ = Describe("GET advertisement list equal-timestamp pagination", func() {
	It("continues deterministically through equal-timestamp records spanning DynamoDB response pages", func() {
		ids := []string{"0192f8a0-7b00-0000-0000-000000000921", "0192f8a0-7b00-0000-0000-000000000922", "0192f8a0-7b00-0000-0000-000000000923"}
		largeContent := strings.Repeat("x", 360*1024)
		for _, id := range ids {
			Expect(putListAdvertisementWithContent(adsListTable, id, string(adfeature.StatusActive), 5000, largeContent)).To(Succeed())
		}
		repository := addb.NewConfiguredRepository(userIntegrationSuite.Client, platformconfig.AdsConfig{AdsTable: adsListTable, CursorKey: "integration-test-cursor-key", CursorKeyID: "test", CursorVersion: 1})
		first, err := repository.List(context.Background(), adfeature.ListQuery{Status: adfeature.StatusActive, Limit: 2, UserID: "0192f8a0-7b00-7000-8000-000000000999"})
		Expect(err).NotTo(HaveOccurred())
		Expect(first.Items).To(HaveLen(2))
		Expect(first.Items[0].ID).To(Equal(ids[2]))
		Expect(first.Items[1].ID).To(Equal(ids[1]))

		second, err := repository.List(context.Background(), adfeature.ListQuery{Status: adfeature.StatusActive, Limit: 2, Cursor: first.NextCursor, UserID: "0192f8a0-7b00-7000-8000-000000000999"})
		Expect(err).NotTo(HaveOccurred())
		Expect(second.Items).To(HaveLen(1))
		Expect(second.Items[0].ID).To(Equal(ids[0]))
	})
})

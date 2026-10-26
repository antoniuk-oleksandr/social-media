package dynamodb

import (
	"context"
	"errors"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/ad"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("GetByID", func() {
	const id = "0192f8a0-7b00-7000-8000-000000000901"

	It("constructs a repository with the configured table", func() {
		repository := NewRepository(nil, "ads-test")
		Expect(repository).NotTo(BeNil())
		Expect(repository.(*Repository).TableName).To(Equal("ads-test"))
	})

	item := map[string]types.AttributeValue{
		"adId": &types.AttributeValueMemberS{Value: id}, "entityType": &types.AttributeValueMemberS{Value: "ad"},
		"advertiser": &types.AttributeValueMemberS{Value: "Example"}, "content": &types.AttributeValueMemberS{Value: "hello"},
		"targetUrl": &types.AttributeValueMemberS{Value: "https://example.com"}, "startsAt": &types.AttributeValueMemberS{Value: "2026-10-18T11:00:00Z"},
		"endsAt": &types.AttributeValueMemberS{Value: "2026-10-18T13:00:00Z"}, "status": &types.AttributeValueMemberS{Value: "active"},
		"createdBy": &types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000902"}, "createdAt": &types.AttributeValueMemberS{Value: "2026-10-18T12:00:00.123Z"},
		"createdAtEpoch": &types.AttributeValueMemberN{Value: "1792324800123"},
	}

	It("reads and maps the complete advertisement", func() {
		api := &putItemAPI{getOutput: &dynamodb.GetItemOutput{Item: item}}
		value, err := (&Repository{Client: api, TableName: "ads-test"}).GetByID(context.Background(), id)

		Expect(err).NotTo(HaveOccurred())
		Expect(*api.getInput.TableName).To(Equal("ads-test"))
		Expect(api.getInput.Key["adId"]).To(Equal(&types.AttributeValueMemberS{Value: id}))
		Expect(value.ID).To(Equal(id))
		Expect(value.Status).To(Equal(ad.StatusActive))
		Expect(value.CreatedAt).To(Equal(time.Date(2026, 10, 18, 12, 0, 0, 123000000, time.UTC)))
		Expect(value.CreatedAtEpoch).To(Equal(int64(1792324800123)))
	})

	It("maps an absent item to not found", func() {
		value, err := (&Repository{Client: &putItemAPI{getOutput: &dynamodb.GetItemOutput{}}, TableName: "ads-test"}).GetByID(context.Background(), id)
		Expect(value).To(Equal(ad.Advertisement{}))
		Expect(err).To(MatchError(ad.ErrNotFound))
	})

	It("translates malformed stored timestamps and numbers", func() {
		for _, name := range []string{"startsAt", "endsAt", "createdAt", "createdAtEpoch"} {
			malformed := cloneItem(item)
			if name == "createdAtEpoch" {
				malformed[name] = &types.AttributeValueMemberN{Value: "not-a-number"}
			} else {
				malformed[name] = &types.AttributeValueMemberS{Value: "not-a-timestamp"}
			}
			value, err := (&Repository{Client: &putItemAPI{getOutput: &dynamodb.GetItemOutput{Item: malformed}}, TableName: "ads-test"}).GetByID(context.Background(), id)
			Expect(value).To(Equal(ad.Advertisement{}), name)
			Expect(err).To(MatchError(ad.ErrDependencyUnavailable), name)
		}
	})

	It("translates read failures and malformed records to dependency unavailable", func() {
		for _, api := range []*putItemAPI{
			{err: errors.New("DynamoDB unavailable")},
			{getOutput: &dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{"adId": &types.AttributeValueMemberS{Value: id}}}},
		} {
			value, err := (&Repository{Client: api, TableName: "ads-test"}).GetByID(context.Background(), id)
			Expect(value).To(Equal(ad.Advertisement{}))
			Expect(err).To(MatchError(ad.ErrDependencyUnavailable))
		}
	})
})

func cloneItem(source map[string]types.AttributeValue) map[string]types.AttributeValue {
	copy := make(map[string]types.AttributeValue, len(source))
	for key, value := range source {
		copy[key] = value
	}
	return copy
}

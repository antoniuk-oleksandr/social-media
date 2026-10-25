package dynamodb

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/ad"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestRepositorySuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Advertisement DynamoDB repository suite")
}

type putItemAPI struct {
	input *dynamodb.PutItemInput
	err   error
}

func (f *putItemAPI) PutItem(_ context.Context, input *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
	f.input = input
	return &dynamodb.PutItemOutput{}, f.err
}

var _ = Describe("Create", func() {
	value := ad.Advertisement{
		ID: "0192f8a0-7b00-7000-8000-000000000901", Type: "ad", Advertiser: "Example", Content: "hello", TargetURL: "https://example.com",
		StartsAt: time.Date(2026, 10, 18, 11, 0, 0, 0, time.UTC), EndsAt: time.Date(2026, 10, 18, 13, 0, 0, 0, time.UTC), Status: ad.StatusActive,
		CreatedBy: "0192f8a0-7b00-7000-8000-000000000902", CreatedAt: time.Date(2026, 10, 18, 12, 0, 0, 123000000, time.UTC), CreatedAtEpoch: 1792326900123,
	}

	It("writes the complete mapped item", func() {
		api := &putItemAPI{}
		err := (&Repository{Client: api, TableName: "ads-test"}).Create(context.Background(), value)
		Expect(err).NotTo(HaveOccurred())
		Expect(*api.input.TableName).To(Equal("ads-test"))
		Expect(api.input.ConditionExpression).NotTo(BeNil())
		Expect(api.input.Item["adId"]).To(Equal(&types.AttributeValueMemberS{Value: value.ID}))
		Expect(api.input.Item["startsAtEpoch"]).To(Equal(&types.AttributeValueMemberN{Value: "1792321200000"}))
		Expect(api.input.Item["createdAtEpoch"]).To(Equal(&types.AttributeValueMemberN{Value: "1792326900123"}))
	})

	It("maps DynamoDB failures to dependency unavailable", func() {
		api := &putItemAPI{err: errors.New("dynamodb unavailable")}
		err := (&Repository{Client: api, TableName: "ads-test"}).Create(context.Background(), value)
		Expect(err).To(MatchError(ad.ErrDependencyUnavailable))
	})
})

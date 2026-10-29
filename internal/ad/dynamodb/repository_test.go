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
	"go.uber.org/mock/gomock"
)

func TestRepositorySuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Advertisement DynamoDB repository suite")
}

type putItemAPIFixture struct {
	*MockAPI
	input        *dynamodb.PutItemInput
	getInput     *dynamodb.GetItemInput
	getOutput    *dynamodb.GetItemOutput
	deleteInput  *dynamodb.DeleteItemInput
	queryInput   *dynamodb.QueryInput
	queryOutput  *dynamodb.QueryOutput
	queryOutputs []*dynamodb.QueryOutput
	queryCalls   int
	err          error
}

func newPutItemAPIFixture(err error) *putItemAPIFixture {
	f := &putItemAPIFixture{MockAPI: NewMockAPI(gomock.NewController(GinkgoT())), err: err}
	f.EXPECT().PutItem(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(_ context.Context, in *dynamodb.PutItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.PutItemOutput, error) {
		f.input = in
		return &dynamodb.PutItemOutput{}, f.err
	})
	f.EXPECT().GetItem(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(_ context.Context, in *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
		f.getInput = in
		if f.getOutput != nil {
			return f.getOutput, f.err
		}
		return &dynamodb.GetItemOutput{}, f.err
	})
	f.EXPECT().DeleteItem(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(_ context.Context, in *dynamodb.DeleteItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.DeleteItemOutput, error) {
		f.deleteInput = in
		return &dynamodb.DeleteItemOutput{}, f.err
	})
	f.EXPECT().Query(gomock.Any(), gomock.Any(), gomock.Any()).AnyTimes().DoAndReturn(func(_ context.Context, in *dynamodb.QueryInput, _ ...func(*dynamodb.Options)) (*dynamodb.QueryOutput, error) {
		f.queryInput = in
		f.queryCalls++
		if len(f.queryOutputs) >= f.queryCalls {
			return f.queryOutputs[f.queryCalls-1], f.err
		}
		if f.queryCalls > 1 {
			return &dynamodb.QueryOutput{}, f.err
		}
		if f.queryOutput != nil {
			return f.queryOutput, f.err
		}
		return &dynamodb.QueryOutput{}, f.err
	})
	return f
}

func newPutItemAPIQuery(output *dynamodb.QueryOutput, outputs []*dynamodb.QueryOutput, err error) *putItemAPIFixture {
	f := newPutItemAPIFixture(err)
	f.queryOutput = output
	f.queryOutputs = outputs
	return f
}

var _ = Describe("Create", func() {
	value := ad.Advertisement{
		ID: "0192f8a0-7b00-7000-8000-000000000901", Type: "ad", Advertiser: "Example", Content: "hello", TargetURL: "https://example.com",
		StartsAt: time.Date(2026, 10, 18, 11, 0, 0, 0, time.UTC), EndsAt: time.Date(2026, 10, 18, 13, 0, 0, 0, time.UTC), Status: ad.StatusActive,
		CreatedBy: "0192f8a0-7b00-7000-8000-000000000902", CreatedAt: time.Date(2026, 10, 18, 12, 0, 0, 123000000, time.UTC), CreatedAtEpoch: 1792326900123,
	}

	It("writes the complete mapped item", func() {
		api := newPutItemAPIFixture(nil)
		err := (&Repository{Client: api, TableName: "ads-test"}).Create(context.Background(), value)
		Expect(err).NotTo(HaveOccurred())
		Expect(*api.input.TableName).To(Equal("ads-test"))
		Expect(api.input.ConditionExpression).NotTo(BeNil())
		Expect(api.input.Item["adId"]).To(Equal(&types.AttributeValueMemberS{Value: value.ID}))
		Expect(api.input.Item["startsAtEpoch"]).To(Equal(&types.AttributeValueMemberN{Value: "1792321200000"}))
		Expect(api.input.Item["createdAtEpoch"]).To(Equal(&types.AttributeValueMemberN{Value: "1792326900123"}))
	})

	It("maps DynamoDB failures to dependency unavailable", func() {
		api := newPutItemAPIFixture(errors.New("dynamodb unavailable"))
		err := (&Repository{Client: api, TableName: "ads-test"}).Create(context.Background(), value)
		Expect(err).To(MatchError(ad.ErrDependencyUnavailable))
	})
})

var _ = Describe("Update", func() {
	value := ad.Advertisement{
		ID: "0192f8a0-7b00-0000-0000-000000000901", Type: "ad", Advertiser: "Example", Content: "hello", TargetURL: "https://example.com",
		StartsAt: time.Date(2026, 10, 18, 11, 0, 0, 0, time.UTC), EndsAt: time.Date(2026, 10, 18, 13, 0, 0, 0, time.UTC), Status: ad.StatusActive,
		CreatedBy: "0192f8a0-7b00-0000-0000-000000000902", CreatedAt: time.Date(2026, 10, 18, 12, 0, 0, 123000000, time.UTC), CreatedAtEpoch: 1792326900123,
	}

	It("maps a failed existence condition to not found", func() {
		api := newPutItemAPIFixture(&types.ConditionalCheckFailedException{})
		err := (&Repository{Client: api, TableName: "ads-test"}).Update(context.Background(), value)
		Expect(*api.input.ConditionExpression).To(Equal("attribute_exists(adId)"))
		Expect(err).To(MatchError(ad.ErrNotFound))
	})

	It("maps other DynamoDB failures to dependency unavailable", func() {
		api := newPutItemAPIFixture(errors.New("dynamodb unavailable"))
		err := (&Repository{Client: api, TableName: "ads-test"}).Update(context.Background(), value)
		Expect(err).To(MatchError(ad.ErrDependencyUnavailable))
	})
})

var _ = Describe("Delete", func() {
	It("deletes by ad ID with an existence condition", func() {
		api := newPutItemAPIFixture(nil)
		err := (&Repository{Client: api, TableName: "ads-test"}).Delete(context.Background(), "0192f8a0-7b00-7000-8000-000000000901")
		Expect(err).NotTo(HaveOccurred())
		Expect(*api.deleteInput.TableName).To(Equal("ads-test"))
		Expect(api.deleteInput.Key["adId"]).To(Equal(&types.AttributeValueMemberS{Value: "0192f8a0-7b00-7000-8000-000000000901"}))
		Expect(*api.deleteInput.ConditionExpression).To(Equal("attribute_exists(adId)"))
	})

	It("maps a failed existence condition to not found", func() {
		api := newPutItemAPIFixture(&types.ConditionalCheckFailedException{})
		err := (&Repository{Client: api, TableName: "ads-test"}).Delete(context.Background(), "0192f8a0-7b00-7000-8000-000000000901")
		Expect(err).To(MatchError(ad.ErrNotFound))
	})

	It("maps other DynamoDB failures to dependency unavailable", func() {
		api := newPutItemAPIFixture(errors.New("dynamodb unavailable"))
		err := (&Repository{Client: api, TableName: "ads-test"}).Delete(context.Background(), "0192f8a0-7b00-7000-8000-000000000901")
		Expect(err).To(MatchError(ad.ErrDependencyUnavailable))
	})
})

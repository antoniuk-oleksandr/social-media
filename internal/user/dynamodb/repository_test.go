package dynamodb

import (
	"context"
	"errors"

	"github.com/antoniuk-oleksandr/social-media/internal/user"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
)

const repositoryTestID = "0192f8a0-7b00-7000-8000-000000000001"

var _ = Describe("DynamoDB user repository", func() {
	It("uses a String key and maps a complete UUIDv7 user record", func() {
		client := dynamodb.New(dynamodb.Options{})
		Expect(NewRepository(client, "users", "uniques")).NotTo(BeNil())
		Expect(NewConfiguredRepository(client, "users", "uniques")).NotTo(BeNil())
		ctrl := gomock.NewController(GinkgoT())
		api := NewMockAPI(ctrl)
		var gotInput *dynamodb.GetItemInput
		api.EXPECT().GetItem(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, input *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
			gotInput = input
			return &dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{
				"userId": &types.AttributeValueMemberS{Value: repositoryTestID}, "username": &types.AttributeValueMemberS{Value: "alice"},
				"email": &types.AttributeValueMemberS{Value: "alice@example.com"}, "displayName": &types.AttributeValueMemberS{Value: "Alice"},
				"role": &types.AttributeValueMemberS{Value: "user"}, "status": &types.AttributeValueMemberS{Value: "active"},
				"createdAt": &types.AttributeValueMemberS{Value: "2026-10-24T12:00:00Z"},
			}}, nil
		})
		got, err := (&Repository{Client: api, TableName: "users-test"}).GetByID(context.Background(), repositoryTestID)
		Expect(err).NotTo(HaveOccurred())
		Expect(got.ID).To(Equal(repositoryTestID))
		Expect(gotInput.Key["userId"]).To(Equal(&types.AttributeValueMemberS{Value: repositoryTestID}))
	})

	DescribeTable("rejects non-canonical UUIDs before DynamoDB", func(id string) {
		ctrl := gomock.NewController(GinkgoT())
		api := NewMockAPI(ctrl)
		api.EXPECT().GetItem(gomock.Any(), gomock.Any()).AnyTimes().Return(nil, nil)
		_, err := (&Repository{Client: api, TableName: "users-test"}).GetByID(context.Background(), id)
		Expect(err).To(MatchError(user.ErrInvalidID))
	}, Entry("numeric", "123"), Entry("wrong version", "0192f8a0-7b00-6000-8000-000000000001"), Entry("uppercase", "0192F8A0-7B00-7000-8000-000000000001"))

	It("maps missing, dependency, and malformed records", func() {
		ctrl := gomock.NewController(GinkgoT())
		api := NewMockAPI(ctrl)
		api.EXPECT().GetItem(gomock.Any(), gomock.Any()).Return(&dynamodb.GetItemOutput{}, nil)
		_, err := (&Repository{Client: api, TableName: "users-test"}).GetByID(context.Background(), repositoryTestID)
		Expect(err).To(MatchError(user.ErrNotFound))
		api = NewMockAPI(gomock.NewController(GinkgoT()))
		api.EXPECT().GetItem(gomock.Any(), gomock.Any()).Return(nil, errors.New("network"))
		_, err = (&Repository{Client: api, TableName: "users-test"}).GetByID(context.Background(), repositoryTestID)
		Expect(err).To(MatchError(user.ErrDependencyUnavailable))
		api = NewMockAPI(gomock.NewController(GinkgoT()))
		api.EXPECT().GetItem(gomock.Any(), gomock.Any()).Return(&dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{"userId": &types.AttributeValueMemberS{Value: repositoryTestID}, "createdAt": &types.AttributeValueMemberS{Value: "bad"}}}, nil)
		_, err = (&Repository{Client: api, TableName: "users-test"}).GetByID(context.Background(), repositoryTestID)
		Expect(err).To(MatchError(user.ErrDependencyUnavailable))
	})

	It("translates a non-conditional UpdateItem failure to a dependency error", func() {
		ctrl := gomock.NewController(GinkgoT())
		api := NewMockAPI(ctrl)
		var updateInput *dynamodb.UpdateItemInput
		api.EXPECT().GetItem(gomock.Any(), gomock.Any()).Return(&dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{
			"userId": &types.AttributeValueMemberS{Value: repositoryTestID}, "username": &types.AttributeValueMemberS{Value: "alice"},
			"createdAt": &types.AttributeValueMemberS{Value: "2026-10-24T12:00:00Z"},
		}}, nil)
		api.EXPECT().UpdateItem(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, input *dynamodb.UpdateItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.UpdateItemOutput, error) {
			updateInput = input
			return nil, errors.New("throttled")
		})
		displayName := "Alice Updated"
		_, err := (&Repository{Client: api, TableName: "users-test"}).UpdateProfile(context.Background(), user.UpdateProfileCommand{UserID: repositoryTestID, DisplayName: &displayName})

		Expect(err).To(MatchError(user.ErrDependencyUnavailable))
		Expect(updateInput).NotTo(BeNil())
		Expect(*updateInput.ConditionExpression).To(Equal("attribute_exists(userId)"))
	})
})

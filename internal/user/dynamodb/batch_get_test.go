package dynamodb

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
)

var _ = Describe("batch user lookup", func() {
	It("loads unique authors with one BatchGetItem request", func() {
		firstID := "0192f8a0-7b00-7000-8000-000000000001"
		secondID := "0192f8a0-7b00-7000-8000-000000000002"
		ctrl := gomock.NewController(GinkgoT())
		api := NewMockAPI(ctrl)
		var calls int
		var inputs []*dynamodb.BatchGetItemInput
		items := []map[string]types.AttributeValue{
			userItem(firstID, "alice"),
			userItem(secondID, "bob"),
		}
		api.EXPECT().BatchGetItem(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, input *dynamodb.BatchGetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.BatchGetItemOutput, error) {
			calls++
			inputs = append(inputs, input)
			return &dynamodb.BatchGetItemOutput{Responses: map[string][]map[string]types.AttributeValue{"users": items}}, nil
		})
		repository := &Repository{Client: api, TableName: "users"}

		users, err := repository.GetByIDs(context.Background(), []string{firstID, secondID, firstID})

		Expect(err).NotTo(HaveOccurred())
		Expect(users).To(HaveLen(2))
		Expect(calls).To(Equal(1))
		Expect(inputs[0].RequestItems["users"].Keys).To(HaveLen(2))
	})
})

func userItem(id, username string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{
		"userId":      &types.AttributeValueMemberS{Value: id},
		"username":    &types.AttributeValueMemberS{Value: username},
		"email":       &types.AttributeValueMemberS{Value: username + "@example.com"},
		"displayName": &types.AttributeValueMemberS{Value: username},
		"role":        &types.AttributeValueMemberS{Value: "user"},
		"status":      &types.AttributeValueMemberS{Value: "active"},
		"createdAt":   &types.AttributeValueMemberS{Value: "2026-10-26T00:00:00Z"},
	}
}

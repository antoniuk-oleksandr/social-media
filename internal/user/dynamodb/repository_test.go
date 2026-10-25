package dynamodb

import (
	"context"
	"errors"

	"github.com/antoniuk-oleksandr/social-media/internal/user"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type getItemAPI struct {
	output *dynamodb.GetItemOutput
	err    error
	input  *dynamodb.GetItemInput
}

func (f *getItemAPI) GetItem(_ context.Context, input *dynamodb.GetItemInput, _ ...func(*dynamodb.Options)) (*dynamodb.GetItemOutput, error) {
	f.input = input
	return f.output, f.err
}

const repositoryTestID = "0192f8a0-7b00-7000-8000-000000000001"

var _ = Describe("DynamoDB user repository", func() {
	It("uses a String key and maps a complete UUIDv7 user record", func() {
		client := dynamodb.New(dynamodb.Options{})
		Expect(NewRepository(client, "users")).NotTo(BeNil())
		Expect(NewConfiguredRepository(client, "users")).NotTo(BeNil())
		api := &getItemAPI{output: &dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{
			"userId": &types.AttributeValueMemberS{Value: repositoryTestID}, "username": &types.AttributeValueMemberS{Value: "alice"},
			"email": &types.AttributeValueMemberS{Value: "alice@example.com"}, "displayName": &types.AttributeValueMemberS{Value: "Alice"},
			"role": &types.AttributeValueMemberS{Value: "user"}, "status": &types.AttributeValueMemberS{Value: "active"},
			"createdAt": &types.AttributeValueMemberS{Value: "2026-10-24T12:00:00Z"},
		}}}
		got, err := (&Repository{Client: api, TableName: "users-test"}).GetByID(context.Background(), repositoryTestID)
		Expect(err).NotTo(HaveOccurred())
		Expect(got.ID).To(Equal(repositoryTestID))
		Expect(api.input.Key["userId"]).To(Equal(&types.AttributeValueMemberS{Value: repositoryTestID}))
	})

	DescribeTable("rejects non-canonical UUIDs before DynamoDB", func(id string) {
		api := &getItemAPI{}
		_, err := (&Repository{Client: api, TableName: "users-test"}).GetByID(context.Background(), id)
		Expect(err).To(MatchError(user.ErrInvalidID))
		Expect(api.input).To(BeNil())
	}, Entry("numeric", "123"), Entry("wrong version", "0192f8a0-7b00-6000-8000-000000000001"), Entry("uppercase", "0192F8A0-7B00-7000-8000-000000000001"))

	It("maps missing, dependency, and malformed records", func() {
		_, err := (&Repository{Client: &getItemAPI{output: &dynamodb.GetItemOutput{}}, TableName: "users-test"}).GetByID(context.Background(), repositoryTestID)
		Expect(err).To(MatchError(user.ErrNotFound))
		_, err = (&Repository{Client: &getItemAPI{err: errors.New("network")}, TableName: "users-test"}).GetByID(context.Background(), repositoryTestID)
		Expect(err).To(MatchError(user.ErrDependencyUnavailable))
		_, err = (&Repository{Client: &getItemAPI{output: &dynamodb.GetItemOutput{Item: map[string]types.AttributeValue{"userId": &types.AttributeValueMemberS{Value: repositoryTestID}, "createdAt": &types.AttributeValueMemberS{Value: "bad"}}}}, TableName: "users-test"}).GetByID(context.Background(), repositoryTestID)
		Expect(err).To(MatchError(user.ErrDependencyUnavailable))
	})
})

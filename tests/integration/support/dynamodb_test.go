package support

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDynamoDBSupport(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DynamoDB support suite")
}

var _ = Describe("DynamoDB support", func() {
	It("attempts to terminate the configured container on close", func() {
		Expect(func() {
			Expect((&DynamoDBSuite{}).Close(context.Background())).To(Succeed())
		}).To(Panic())
	})

	It("returns a client error when CreateTable cannot connect", func() {
		cfg, err := config.LoadDefaultConfig(context.Background(),
			config.WithRegion("us-east-1"),
			config.WithBaseEndpoint("http://127.0.0.1:1"),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
		)
		Expect(err).NotTo(HaveOccurred())
		client := dynamodb.NewFromConfig(cfg)
		Expect(CreateTable(context.Background(), client, "test-table", "id")).To(HaveOccurred())
	})

	It("uses the connection string as the DynamoDB endpoint host and port", func() {
		Expect(dynamoDBEndpoint("127.0.0.1:8000")).To(Equal("http://127.0.0.1:8000"))
	})

	It("returns an error for a canceled context without a suite", func() {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		suite, err := StartDynamoDB(ctx, "test")
		if suite != nil {
			Expect(suite.Close(context.Background())).To(Succeed())
		}
		Expect(err).To(HaveOccurred())
		Expect(suite).To(BeNil())
	})
})

package dynamodb

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestFeedDynamoDBGinkgo(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Feed DynamoDB unit suite")
}

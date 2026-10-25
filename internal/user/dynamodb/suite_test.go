package dynamodb

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestDynamoDBSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "user DynamoDB suite")
}

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/antoniuk-oleksandr/social-media/tests/integration/support"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// All integration specs in this directory intentionally share one process,
// DynamoDB Local container, and core table set. Individual feature suites use
// separate tables where their access patterns require different schemas.
var integrationSuite *support.DynamoDBSuite
var userIntegrationSuite *support.DynamoDBSuite
var feedSuite *support.DynamoDBSuite

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "integration suite")
}

var _ = BeforeSuite(func() {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var err error
	integrationSuite, err = support.StartDynamoDB(ctx, "tests-integration")
	Expect(err).NotTo(HaveOccurred(), "DynamoDB integration suite must start")

	// Keep the existing feature-level names as aliases while every package is
	// executed in this one Go test process.
	userIntegrationSuite = integrationSuite
	feedSuite = integrationSuite
	setupFeedTables()
	setupAdsListTable()
})

var _ = AfterSuite(func() {
	if integrationSuite != nil {
		Expect(integrationSuite.Close(context.Background())).To(Succeed())
	}
})

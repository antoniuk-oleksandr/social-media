package dev

import (
	"os"
	"strings"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestFeedTerraformContract(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Feed Terraform contract")
}

var _ = Describe("feed deployment contract", func() {
	var lambda, iam string
	BeforeEach(func() {
		lambdaBytes, readErr := os.ReadFile("lambda.tf")
		Expect(readErr).NotTo(HaveOccurred())
		lambda = string(lambdaBytes)
		iamBytes, readErr := os.ReadFile("iam.tf")
		Expect(readErr).NotTo(HaveOccurred())
		iam = string(iamBytes)
	})

	It("configures the feed Lambda with its table environment variables", func() {
		feed := lambda[strings.Index(lambda, `resource "aws_lambda_function" "feed"`):]
		Expect(feed).To(ContainSubstring("ADS_TABLE"))
		Expect(feed).To(ContainSubstring("POSTS_TABLE"))
		Expect(feed).To(ContainSubstring("USERS_TABLE"))
		Expect(feed).To(ContainSubstring("aws_iam_role_policy.ads_lambda"))
	})

	It("grants the feed role only the required read permissions", func() {
		feed := iam[strings.Index(iam, `data "aws_iam_policy_document" "ads_lambda"`):]
		Expect(feed).To(ContainSubstring("dynamodb:DescribeTable"))
		Expect(feed).To(ContainSubstring("dynamodb:BatchGetItem"))
		Expect(feed).To(ContainSubstring("dynamodb:GetItem"))
		Expect(feed).To(ContainSubstring("aws_dynamodb_table.users.arn"))
		Expect(feed).To(ContainSubstring("aws_dynamodb_table.posts.arn"))
	})
})

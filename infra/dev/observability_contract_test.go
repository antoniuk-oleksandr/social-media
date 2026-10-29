package dev

import (
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("observability deployment contract", func() {
	It("configures every Lambda for AWS ADOT and X-Ray tracing", func() {
		content, err := os.ReadFile("../modules/social-media/lambda.tf")
		Expect(err).NotTo(HaveOccurred())
		lambda := string(content)
		for _, function := range []string{"auth", "users", "posts", "ads", "feed"} {
			start := strings.Index(lambda, `resource "aws_lambda_function" "`+function+`"`)
			Expect(start).To(BeNumerically(">=", 0), function)
			end := strings.Index(lambda[start+1:], "\nresource ")
			if end < 0 {
				end = len(lambda) - start - 1
			}
			block := lambda[start : start+1+end]
			Expect(block).To(ContainSubstring("layers"), function)
			Expect(block).To(ContainSubstring("[var.adot_layer_arn]"), function)
			Expect(block).To(ContainSubstring(`mode = "Active"`), function)
			Expect(block).To(ContainSubstring("APP_ENV"), function)
			Expect(block).To(ContainSubstring("var.environment"), function)
			Expect(block).NotTo(ContainSubstring("127.0.0.1:4318"), function)
			Expect(block).NotTo(ContainSubstring("OTEL_EXPORTER_OTLP_ENDPOINT"), function)
			Expect(block).To(ContainSubstring("OTEL_TRACES_SAMPLER_ARG"), function)
		}
	})

	It("declares metric filters, alarms, dashboards, and AWS tracing", func() {
		cloudwatch, err := os.ReadFile("../modules/social-media/cloudwatch.tf")
		Expect(err).NotTo(HaveOccurred())
		Expect(string(cloudwatch)).To(ContainSubstring(`resource "aws_cloudwatch_dashboard" "main"`))
		Expect(string(cloudwatch)).To(ContainSubstring(`resource "aws_cloudwatch_log_metric_filter" "application_errors"`))
		Expect(string(cloudwatch)).To(ContainSubstring(`resource "aws_cloudwatch_metric_alarm" "lambda_errors"`))
	})
})

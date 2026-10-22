package main

import (
	"context"
	"errors"
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"os"
)

var _ = Describe("InitTablesPropagatesPrintError", func() {
	It("runs the test", func() {
		client := &fakeClient{}
		want := errors.New("print")
		if err := initTables(nil, client, func(string, ...any) (int, error) { return 0, want }); !errors.Is(err, want) {
			Expect(false).To(BeTrue(), fmt.Sprintf("got %v, want %v", err, want))
		}

	})
})

var _ = Describe("LoadConfig", func() {
	It("runs the test", func() {
		cfg, err := loadConfig(context.Background(), "us-east-1")
		if err != nil || cfg.Region != "us-east-1" {
			Expect(false).To(BeTrue(), fmt.Sprintf("cfg=%q err=%v", cfg.Region, err))
		}

	})
})

var _ = Describe("MainReportsDynamoDBStartupFailure", func() {
	It("runs the test", func() {
		os.Setenv("AWS_REGION", "us-east-1")
		DeferCleanup(func() { _ = os.Unsetenv("AWS_REGION") })
		os.Setenv("AWS_ACCESS_KEY_ID", "dummy")
		DeferCleanup(func() { _ = os.Unsetenv("AWS_ACCESS_KEY_ID") })
		os.Setenv("AWS_SECRET_ACCESS_KEY", "dummy")
		DeferCleanup(func() { _ = os.Unsetenv("AWS_SECRET_ACCESS_KEY") })
		os.Setenv("DYNAMODB_ENDPOINT", "http://127.0.0.1:1")
		DeferCleanup(func() { _ = os.Unsetenv("DYNAMODB_ENDPOINT") })

		defer func() {
			if recover() == nil {
				Expect(false).To(BeTrue(), fmt.Sprint("main() did not report the expected DynamoDB startup failure"))
			}
		}()
		main()

	})
})

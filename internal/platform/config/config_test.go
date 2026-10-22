package config

import (
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"os"
)

var _ = Describe("LoadUsesConfiguredOrDefaultRegion", func() {
	It("runs the test", func() {
		os.Unsetenv("AWS_REGION")
		cfg, err := Load()
		if err != nil || cfg.Region != "us-east-1" {
			Expect(false).To(BeTrue(), fmt.Sprintf("default region: cfg=%q err=%v", cfg.Region, err))
		}
		os.Setenv("AWS_REGION", "eu-west-1")
		defer os.Unsetenv("AWS_REGION")
		cfg, err = Load()
		if err != nil || cfg.Region != "eu-west-1" {
			Expect(false).To(BeTrue(), fmt.Sprintf("configured region: cfg=%q err=%v", cfg.Region, err))
		}

	})
})

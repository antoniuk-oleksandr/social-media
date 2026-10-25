package config

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("LoadAdsConfig", func() {
	It("loads configured names and uses isolated defaults", func() {
		previous, present := os.LookupEnv("ADS_TABLE")
		DeferCleanup(func() {
			if present {
				Expect(os.Setenv("ADS_TABLE", previous)).To(Succeed())
			} else {
				Expect(os.Unsetenv("ADS_TABLE")).To(Succeed())
			}
		})
		Expect(os.Setenv("ADS_TABLE", "configured-ads")).To(Succeed())
		Expect(LoadAdsConfig()).To(Equal(AdsConfig{AdsTable: "configured-ads"}))
		Expect(os.Unsetenv("ADS_TABLE")).To(Succeed())
		Expect(LoadAdsConfig()).To(Equal(AdsConfig{AdsTable: "social-media-local-ads"}))
	})
})

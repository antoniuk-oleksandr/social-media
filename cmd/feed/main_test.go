package main

import (
	"context"
	"net/http"
	"os"

	feedfx "github.com/antoniuk-oleksandr/social-media/internal/feed/fx"
	platformconfig "github.com/antoniuk-oleksandr/social-media/internal/platform/config"
	platformfx "github.com/antoniuk-oleksandr/social-media/internal/platform/fx"
	platformlogging "github.com/antoniuk-oleksandr/social-media/internal/platform/logging"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/fx"
)

var _ = Describe("feed composition graph", func() {
	It("builds with the configured dependencies", func() {
		previous := map[string]string{}
		for key, value := range map[string]string{"APP_ENV": "local", "JWT_KID": "test-kid", "JWT_ACCESS_SECRET": "test-secret", "ADS_CURSOR_KEY": "cursor-secret", "DYNAMODB_ENDPOINT": "http://127.0.0.1:8000"} {
			previous[key] = os.Getenv(key)
			Expect(os.Setenv(key, value)).To(Succeed())
		}
		DeferCleanup(func() {
			for key, value := range previous {
				if value == "" {
					Expect(os.Unsetenv(key)).To(Succeed())
				} else {
					Expect(os.Setenv(key, value)).To(Succeed())
				}
			}
		})

		var handler http.Handler
		app := fx.New(
			fx.Provide(func() context.Context { return context.Background() }),
			fx.Provide(platformfx.LoadAWSConfig, platformfx.NewConfiguredAdsConfig, platformconfig.LoadPostsConfig, platformconfig.LoadUsersConfig, platformfx.NewDynamoDBClient, platformfx.NewClock, platformlogging.New, platformfx.NewConfiguredAccessTokenIssuer, feedfx.NewPosts, feedfx.NewAds, feedfx.NewAuthors, feedfx.NewService, feedfx.NewHandler, feedfx.NewRouter),
			fx.Populate(&handler),
		)
		Expect(app.Err()).NotTo(HaveOccurred())
		Expect(handler).NotTo(BeNil())
	})
})

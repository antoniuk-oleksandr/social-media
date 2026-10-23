package main

import (
	"context"
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"net/http"

	authfx "github.com/antoniuk-oleksandr/social-media/internal/auth/fx"
	platformfx "github.com/antoniuk-oleksandr/social-media/internal/platform/fx"
	platformlogging "github.com/antoniuk-oleksandr/social-media/internal/platform/logging"
	"github.com/aws/aws-sdk-go-v2/aws"
	"go.uber.org/fx"
)

var _ = Describe("CompositionGraphBuilds", func() {
	It("runs the test", func() {
		var handler http.Handler
		app := fx.New(
			fx.Supply(aws.Config{Region: "test"}),
			fx.Provide(
				platformfx.NewDynamoDBClient,
				platformfx.NewClock,
				platformfx.NewIDGenerator,
				platformfx.NewPasswordHasher,
				platformfx.NewAccessTokenIssuer,
				platformfx.NewOpaqueTokenGenerator,
				platformlogging.New,
				authfx.NewRegistrationStore,
				authfx.NewLoginStore,
				authfx.NewLogoutStore,
				authfx.NewServicePort,
				authfx.NewHandler,
				authfx.NewRouter,
			),
			fx.Populate(&handler),
		)
		if err := app.Err(); err != nil {
			Expect(false).To(BeTrue(), fmt.Sprint(err))
		}
		ctx := context.Background()
		if err := app.Start(ctx); err != nil {
			Expect(false).To(BeTrue(), fmt.Sprint(err))
		}
		if handler == nil {
			Expect(false).To(BeTrue(), fmt.Sprint("handler was not populated"))
		}
		if err := app.Stop(ctx); err != nil {
			Expect(false).To(BeTrue(), fmt.Sprint(err))
		}

	})
})

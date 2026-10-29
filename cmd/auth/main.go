package main

import (
	"context"
	"log"
	"net/http"

	"github.com/akrylysov/algnhsa"
	authfx "github.com/antoniuk-oleksandr/social-media/internal/auth/fx"
	platformconfig "github.com/antoniuk-oleksandr/social-media/internal/platform/config"
	platformfx "github.com/antoniuk-oleksandr/social-media/internal/platform/fx"
	platformlogging "github.com/antoniuk-oleksandr/social-media/internal/platform/logging"
	platformobservability "github.com/antoniuk-oleksandr/social-media/internal/platform/observability"
	"github.com/aws/aws-lambda-go/lambda"
	"go.uber.org/fx"
)

func main() {
	var handler http.Handler

	app := fx.New(
		fx.Provide(
			platformconfig.Load,
			platformfx.NewDynamoDBClient,
			platformfx.NewClock,
			platformfx.NewIDGenerator,
			platformfx.NewPasswordHasher,
			platformfx.NewConfiguredAccessTokenIssuer,
			platformfx.NewOpaqueTokenGenerator,
			platformlogging.New,
			platformobservability.New,
			authfx.NewRegistrationStore,
			authfx.NewLoginStore,
			authfx.NewLogoutStore,
			authfx.NewRefreshStore,
			authfx.NewUserLookup,
			authfx.NewServicePort,
			authfx.NewHandler,
			authfx.NewObservedRouter,
		),
		fx.Populate(&handler),
	)

	ctx := context.Background()
	if err := app.Start(ctx); err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := app.Stop(ctx); err != nil {
			log.Print(err)
		}
	}()

	lambda.StartHandler(platformobservability.WrapLambdaHandler(algnhsa.New(handler, nil)))
}

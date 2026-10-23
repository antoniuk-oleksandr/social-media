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
			authfx.NewRegistrationStore,
			authfx.NewLoginStore,
			authfx.NewLogoutStore,
			authfx.NewServicePort,
			authfx.NewHandler,
			authfx.NewRouter,
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

	lambda.Start(algnhsa.New(handler, nil))
}

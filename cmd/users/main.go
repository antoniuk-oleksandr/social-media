package main

import (
	"context"
	"log"
	"net/http"

	"github.com/akrylysov/algnhsa"
	platformconfig "github.com/antoniuk-oleksandr/social-media/internal/platform/config"
	platformfx "github.com/antoniuk-oleksandr/social-media/internal/platform/fx"
	platformlogging "github.com/antoniuk-oleksandr/social-media/internal/platform/logging"
	platformobservability "github.com/antoniuk-oleksandr/social-media/internal/platform/observability"
	userfx "github.com/antoniuk-oleksandr/social-media/internal/user/fx"
	"github.com/aws/aws-lambda-go/lambda"
	"go.uber.org/fx"
)

func main() {
	var handler http.Handler

	app := fx.New(
		fx.Provide(
			platformconfig.Load,
			platformconfig.LoadUsersConfig,
			platformfx.NewDynamoDBClient,
			platformfx.NewClock,
			platformlogging.New,
			platformobservability.New,
			platformfx.NewConfiguredAccessTokenIssuer,
			userfx.NewConfiguredRepository,
			userfx.NewService,
			userfx.NewHandler,
			userfx.NewObservedRouter,
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

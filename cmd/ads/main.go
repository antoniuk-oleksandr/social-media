package main

import (
	"context"
	"log"
	"net/http"

	"github.com/akrylysov/algnhsa"
	adfx "github.com/antoniuk-oleksandr/social-media/internal/ad/fx"
	platformconfig "github.com/antoniuk-oleksandr/social-media/internal/platform/config"
	platformfx "github.com/antoniuk-oleksandr/social-media/internal/platform/fx"
	platformlogging "github.com/antoniuk-oleksandr/social-media/internal/platform/logging"
	"github.com/aws/aws-lambda-go/lambda"
	"go.uber.org/fx"
)

func main() {
	var handler http.Handler
	app := fx.New(
		fx.Provide(func() context.Context { return context.Background() }),
		fx.Provide(platformfx.LoadAWSConfig, platformconfig.LoadAdsConfig, platformfx.NewDynamoDBClient, platformfx.NewClock, platformfx.NewIDGenerator, platformlogging.New, platformfx.NewConfiguredAccessTokenIssuer, adfx.NewRepository, adfx.NewService, adfx.NewHandler, adfx.NewRouter),
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

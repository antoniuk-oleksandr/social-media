package main

import (
	"context"
	"github.com/akrylysov/algnhsa"
	feedfx "github.com/antoniuk-oleksandr/social-media/internal/feed/fx"
	platformconfig "github.com/antoniuk-oleksandr/social-media/internal/platform/config"
	platformfx "github.com/antoniuk-oleksandr/social-media/internal/platform/fx"
	platformlogging "github.com/antoniuk-oleksandr/social-media/internal/platform/logging"
	"github.com/aws/aws-lambda-go/lambda"
	"go.uber.org/fx"
	"log"
	"net/http"
)

func main() {
	var handler http.Handler
	app := fx.New(fx.Provide(func() context.Context { return context.Background() }, platformfx.LoadAWSConfig, platformfx.NewConfiguredAdsConfig, platformconfig.LoadPostsConfig, platformconfig.LoadUsersConfig, platformfx.NewDynamoDBClient, platformfx.NewClock, platformlogging.New, platformfx.NewConfiguredAccessTokenIssuer, feedfx.NewPosts, feedfx.NewAds, feedfx.NewAuthors, feedfx.NewService, feedfx.NewHandler, feedfx.NewRouter), fx.Populate(&handler))
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

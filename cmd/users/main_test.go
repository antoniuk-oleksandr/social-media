package main

import (
	"context"
	"net/http"
	"os"

	platformfx "github.com/antoniuk-oleksandr/social-media/internal/platform/fx"
	platformlogging "github.com/antoniuk-oleksandr/social-media/internal/platform/logging"
	userfx "github.com/antoniuk-oleksandr/social-media/internal/user/fx"
	"github.com/aws/aws-sdk-go-v2/aws"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/fx"
)

var _ = Describe("Users Lambda composition graph", func() {
	It("constructs and starts the Fx graph", func() {
		previous := os.Getenv("USERS_TABLE")
		Expect(os.Setenv("USERS_TABLE", "users-test")).To(Succeed())
		DeferCleanup(func() { Expect(os.Setenv("USERS_TABLE", previous)).To(Succeed()) })

		var handler http.Handler
		app := fx.New(
			fx.Supply(aws.Config{Region: "us-east-1"}),
			fx.Provide(
				platformfx.NewDynamoDBClient,
				platformlogging.New,
				userfx.NewRepository,
				userfx.NewService,
				userfx.NewHandler,
				userfx.NewRouter,
			),
			fx.Populate(&handler),
		)
		Expect(app.Err()).NotTo(HaveOccurred())
		ctx := context.Background()
		Expect(app.Start(ctx)).To(Succeed())
		DeferCleanup(func() { Expect(app.Stop(ctx)).To(Succeed()) })
		Expect(handler).NotTo(BeNil())
	})
})

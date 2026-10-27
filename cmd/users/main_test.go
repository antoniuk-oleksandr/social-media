package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"time"

	platformconfig "github.com/antoniuk-oleksandr/social-media/internal/platform/config"
	platformfx "github.com/antoniuk-oleksandr/social-media/internal/platform/fx"
	platformjwt "github.com/antoniuk-oleksandr/social-media/internal/platform/jwt"
	platformlogging "github.com/antoniuk-oleksandr/social-media/internal/platform/logging"
	userfeature "github.com/antoniuk-oleksandr/social-media/internal/user"
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
				platformfx.NewAccessTokenIssuer,
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

	It("authenticates a valid configured JWT through the real users graph", func() {
		for key, value := range map[string]string{
			"APP_ENV":           "local",
			"AWS_REGION":        "us-east-1",
			"DYNAMODB_ENDPOINT": "http://127.0.0.1:1",
			"JWT_ACCESS_SECRET": "composition-test-secret",
			"JWT_KID":           "composition-kid",
			"JWT_ISSUER":        "composition-issuer",
			"JWT_AUDIENCE":      "composition-audience",
			"USERS_TABLE":       "composition-users",
			"UNIQUES_TABLE":     "composition-uniques",
		} {
			previous, present := os.LookupEnv(key)
			Expect(os.Setenv(key, value)).To(Succeed())
			DeferCleanup(func() {
				if present {
					Expect(os.Setenv(key, previous)).To(Succeed())
				} else {
					Expect(os.Unsetenv(key)).To(Succeed())
				}
			})
		}

		var handler http.Handler
		var issuer platformjwt.Access
		app := fx.New(
			fx.Supply(aws.Config{Region: "us-east-1"}),
			fx.Provide(
				platformconfig.LoadUsersConfig,
				platformfx.NewDynamoDBClient,
				platformfx.NewClock,
				platformfx.NewConfiguredAccessTokenIssuer,
				platformlogging.New,
				userfx.NewConfiguredRepository,
				userfx.NewService,
				userfx.NewHandler,
				userfx.NewRouter,
			),
			fx.Populate(&handler, &issuer),
		)
		Expect(app.Err()).NotTo(HaveOccurred())
		ctx := context.Background()
		Expect(app.Start(ctx)).To(Succeed())
		DeferCleanup(func() { Expect(app.Stop(ctx)).To(Succeed()) })

		token, err := issuer.Issue(ctx, userfeature.User{ID: "0192f8a0-7b00-7000-8000-000000000001", Role: userfeature.RoleUser}, time.Now())
		Expect(err).NotTo(HaveOccurred())
		request := httptest.NewRequest(http.MethodPatch, "/api/v1/users/me", strings.NewReader(`{"displayName":"Alice"}`))
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)

		Expect(response.Code).To(Equal(http.StatusServiceUnavailable), "the request must pass JWT verification before the unavailable test DynamoDB endpoint")
	})

	It("keeps users Lambda JWT and table environment names in Terraform parity", func() {
		content, err := os.ReadFile("../../infra/modules/social-media/lambda.tf")
		Expect(err).NotTo(HaveOccurred())
		usersStart := strings.Index(string(content), `resource "aws_lambda_function" "users"`)
		Expect(usersStart).To(BeNumerically(">=", 0))
		usersConfig := string(content)[usersStart:]
		for _, name := range []string{"JWT_KID", "JWT_ISSUER", "JWT_AUDIENCE", "JWT_ACCESS_SECRET_PARAMETER", "USERS_TABLE", "UNIQUES_TABLE"} {
			Expect(usersConfig).To(ContainSubstring(name), name)
		}
	})
})

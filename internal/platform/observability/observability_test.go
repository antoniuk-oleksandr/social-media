package observability

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"

	"github.com/antoniuk-oleksandr/social-media/internal/platform/logging"
	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/smithy-go/middleware"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.opentelemetry.io/contrib/propagators/aws/xray"
	"go.opentelemetry.io/otel"
	"go.uber.org/mock/gomock"
)

func newLifecycleMock(ctrl *gomock.Controller) *MockLifecycle {
	lifecycle := NewMockLifecycle(ctrl)
	lifecycle.EXPECT().Append(gomock.Any()).AnyTimes()
	return lifecycle
}

func newLoggerMock(ctrl *gomock.Controller, serverError bool) (*MockLogger, map[string]string) {
	logger := NewMockLogger(ctrl)
	values := make(map[string]string)
	arguments := []any{gomock.Any()}
	for range 9 {
		arguments = append(arguments, gomock.Any())
	}
	var call *gomock.Call
	if serverError {
		call = logger.EXPECT().Error(arguments[0], arguments[1:]...)
	} else {
		call = logger.EXPECT().Info(arguments[0], arguments[1:]...)
	}
	call.DoAndReturn(func(_ string, fields ...logging.Field) error {
		for _, field := range fields {
			if value, ok := field.Value.(string); ok {
				values[field.Key] = value
			}
		}
		return nil
	})
	return logger, values
}

var _ = Describe("HTTP observability middleware", func() {
	It("generates and propagates request and correlation IDs", func() {
		ctrl := gomock.NewController(GinkgoT())
		logger, values := newLoggerMock(ctrl, false)
		lifecycle := newLifecycleMock(ctrl)
		middleware, err := New(lifecycle, logger)
		Expect(err).NotTo(HaveOccurred())

		handler := middleware(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			Expect(RequestID(request.Context())).NotTo(BeEmpty())
			Expect(CorrelationID(request.Context())).To(Equal(RequestID(request.Context())))
			writer.WriteHeader(http.StatusCreated)
			_, err := writer.Write([]byte("created"))
			Expect(err).NotTo(HaveOccurred())
		}))
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", nil)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		Expect(response.Code).To(Equal(http.StatusCreated))
		Expect(response.Header().Get("X-Request-ID")).NotTo(BeEmpty())
		Expect(response.Header().Get("X-Correlation-ID")).To(Equal(response.Header().Get("X-Request-ID")))
		Expect(response.Header().Get("Traceparent")).NotTo(BeEmpty())
		Expect(values["request_id"]).To(Equal(response.Header().Get("X-Request-ID")))
		Expect(values["correlation_id"]).To(Equal(response.Header().Get("X-Correlation-ID")))
		Expect(values["status"]).To(Equal("201"))
		Expect(UserID(WithUserID(request.Context(), "0192f8a0-7b00-7000-8000-000000000001"))).To(Equal("0192f8a0-7b00-7000-8000-000000000001"))
	})

	It("wraps Lambda handlers for local and AWS environments", func() {
		previous := os.Getenv("APP_ENV")
		DeferCleanup(func() {
			Expect(os.Setenv("APP_ENV", previous)).To(Succeed())
		})

		Expect(os.Setenv("APP_ENV", "local")).To(Succeed())
		localProvider, err := newTracerProvider()
		Expect(err).NotTo(HaveOccurred())
		otel.SetTracerProvider(localProvider)
		localHandler := NewMockHandler(gomock.NewController(GinkgoT()))
		localHandler.EXPECT().Invoke(gomock.Any(), gomock.Any()).Return([]byte(`{"ok":true}`), nil)
		_, err = WrapLambdaHandler(localHandler).Invoke(context.Background(), []byte(`{}`))
		Expect(err).NotTo(HaveOccurred())
		Expect(localProvider.Shutdown(context.Background())).To(Succeed())

		Expect(os.Setenv("APP_ENV", "dev")).To(Succeed())
		awsProvider, err := newTracerProvider()
		Expect(err).NotTo(HaveOccurred())
		otel.SetTracerProvider(awsProvider)
		awsHandler := NewMockHandler(gomock.NewController(GinkgoT()))
		awsHandler.EXPECT().Invoke(gomock.Any(), gomock.Any()).Return([]byte(`{"ok":true}`), nil)
		_, err = WrapLambdaHandler(awsHandler).Invoke(context.Background(), []byte(`{}`))
		Expect(err).NotTo(HaveOccurred())
		Expect(awsProvider.Shutdown(context.Background())).To(Succeed())
	})

	It("handles the local Flutter Web CORS preflight", func() {
		previous := os.Getenv("APP_ENV")
		DeferCleanup(func() {
			Expect(os.Setenv("APP_ENV", previous)).To(Succeed())
		})
		Expect(os.Setenv("APP_ENV", "local")).To(Succeed())

		ctrl := gomock.NewController(GinkgoT())
		middleware, err := New(newLifecycleMock(ctrl), NewMockLogger(ctrl))
		Expect(err).NotTo(HaveOccurred())
		request := httptest.NewRequest(http.MethodOptions, "/api/v1/feed", nil)
		request.Header.Set("Origin", localWebOrigin)
		response := httptest.NewRecorder()

		middleware(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			Fail("preflight should not reach the feature router")
		})).ServeHTTP(response, request)

		Expect(response.Code).To(Equal(http.StatusNoContent))
		Expect(response.Header().Get("Access-Control-Allow-Origin")).To(Equal(localWebOrigin))
		Expect(response.Header().Get("Access-Control-Allow-Methods")).To(ContainSubstring("OPTIONS"))
	})

	It("prefers supplied IDs and extracts a Lambda request ID", func() {
		ctrl := gomock.NewController(GinkgoT())
		logger, values := newLoggerMock(ctrl, false)
		middleware, err := New(newLifecycleMock(ctrl), logger)
		Expect(err).NotTo(HaveOccurred())
		request := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
		request.Header.Set("X-Request-ID", "request-from-client")
		request.Header.Set("X-Correlation-ID", "correlation-from-client")
		request = request.WithContext(lambdacontext.NewContext(request.Context(), &lambdacontext.LambdaContext{AwsRequestID: "lambda-request"}))
		response := httptest.NewRecorder()

		middleware(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			Expect(RequestID(request.Context())).To(Equal("request-from-client"))
			Expect(CorrelationID(request.Context())).To(Equal("correlation-from-client"))
			writer.WriteHeader(http.StatusNoContent)
		})).ServeHTTP(response, request)

		Expect(response.Header().Get("X-Request-ID")).To(Equal("request-from-client"))
		Expect(response.Header().Get("X-Correlation-ID")).To(Equal("correlation-from-client"))
		Expect(values["lambda_request_id"]).To(Equal("lambda-request"))
	})

	It("uses a safe default sample rate and preserves incoming trace context", func() {
		ctrl := gomock.NewController(GinkgoT())
		logger, values := newLoggerMock(ctrl, true)
		middleware, err := New(newLifecycleMock(ctrl), logger)
		Expect(err).NotTo(HaveOccurred())
		request := httptest.NewRequest(http.MethodGet, "/health", nil)
		request.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
		response := httptest.NewRecorder()

		middleware(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.WriteHeader(http.StatusInternalServerError)
		})).ServeHTTP(response, request)

		Expect(response.Header().Get("Traceparent")).To(MatchRegexp(`^00-4bf92f3577b34da6a3ce929d0e0e4736-[0-9a-f]{16}-01$`))
		Expect(values["status"]).To(Equal("500"))
		previous := os.Getenv("OTEL_TRACES_SAMPLER_ARG")
		defer os.Setenv("OTEL_TRACES_SAMPLER_ARG", previous)
		Expect(os.Setenv("OTEL_TRACES_SAMPLER_ARG", "not-a-number")).To(Succeed())
		Expect(sampleRate()).To(Equal(0.1))
		Expect(os.Setenv("OTEL_TRACES_SAMPLER_ARG", "0.5")).To(Succeed())
		Expect(sampleRate()).To(Equal(0.5))
	})

	It("traces DynamoDB operations and preserves dependency errors", func() {
		stack := middleware.NewStack("test", func() interface{} { return nil })
		Expect(DynamoDBAPIOption()(stack)).To(Succeed())
		next := middleware.FinalizeHandlerFunc(func(context.Context, middleware.FinalizeInput) (middleware.FinalizeOutput, middleware.Metadata, error) {
			return middleware.FinalizeOutput{}, middleware.Metadata{}, errors.New("dynamodb unavailable")
		})
		_, _, err := newDynamoDBTracingMiddleware().HandleFinalize(context.Background(), middleware.FinalizeInput{}, next)
		Expect(err).To(MatchError("dynamodb unavailable"))
	})

	It("creates an OTLP exporter when explicitly enabled", func() {
		previousEndpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
		previousDisabled := os.Getenv("OTEL_SDK_DISABLED")
		previousAppEnv := os.Getenv("APP_ENV")
		DeferCleanup(func() {
			Expect(os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", previousEndpoint)).To(Succeed())
			Expect(os.Setenv("OTEL_SDK_DISABLED", previousDisabled)).To(Succeed())
			Expect(os.Setenv("APP_ENV", previousAppEnv)).To(Succeed())
		})
		Expect(os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://127.0.0.1:4318")).To(Succeed())
		Expect(os.Setenv("OTEL_SDK_DISABLED", "false")).To(Succeed())
		Expect(os.Setenv("APP_ENV", "local")).To(Succeed())
		provider, err := newTracerProvider()
		Expect(err).NotTo(HaveOccurred())
		Expect(provider.Shutdown(context.Background())).To(Succeed())
	})

	It("uses the AWS X-Ray provider and propagator outside local development", func() {
		previous := os.Getenv("APP_ENV")
		DeferCleanup(func() {
			Expect(os.Setenv("APP_ENV", previous)).To(Succeed())
		})
		Expect(os.Setenv("APP_ENV", "dev")).To(Succeed())

		provider, err := newTracerProvider()
		Expect(err).NotTo(HaveOccurred())
		Expect(provider.Shutdown(context.Background())).To(Succeed())
		Expect(tracePropagator()).To(BeAssignableToTypeOf(xray.Propagator{}))
	})

	It("assigns the configured service name to traces", func() {
		previous := os.Getenv("OTEL_SERVICE_NAME")
		DeferCleanup(func() {
			Expect(os.Setenv("OTEL_SERVICE_NAME", previous)).To(Succeed())
		})
		Expect(os.Setenv("OTEL_SERVICE_NAME", "social-media-local-auth")).To(Succeed())

		appResource, err := newResource()
		Expect(err).NotTo(HaveOccurred())

		attributes := appResource.Attributes()
		var serviceName string
		for _, attribute := range attributes {
			if attribute.Key == "service.name" {
				serviceName = attribute.Value.AsString()
			}
		}
		Expect(serviceName).To(Equal("social-media-local-auth"))
	})
})

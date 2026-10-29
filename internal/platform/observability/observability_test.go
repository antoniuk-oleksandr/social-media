package observability

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"

	"github.com/antoniuk-oleksandr/social-media/internal/platform/logging"
	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/smithy-go/middleware"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/fx"
)

type fakeLifecycle struct {
	hooks []fx.Hook
}

func (l *fakeLifecycle) Append(hook fx.Hook) { l.hooks = append(l.hooks, hook) }

type capturedLog struct {
	message string
	fields  []logging.Field
}

type captureLogger struct {
	mu   sync.Mutex
	logs []capturedLog
}

func (l *captureLogger) Info(message string, fields ...logging.Field) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logs = append(l.logs, capturedLog{message: message, fields: fields})
	return nil
}

func (l *captureLogger) Error(message string, fields ...logging.Field) error {
	return l.Info(message, fields...)
}

func (l *captureLogger) Sync() error { return nil }

func (l *captureLogger) value(key string) string {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, log := range l.logs {
		for _, field := range log.fields {
			if field.Key == key {
				return field.Value.(string)
			}
		}
	}
	return ""
}

var _ = Describe("HTTP observability middleware", func() {
	It("generates and propagates request and correlation IDs", func() {
		logger := &captureLogger{}
		lifecycle := &fakeLifecycle{}
		middleware, err := New(lifecycle, logger)
		Expect(err).NotTo(HaveOccurred())

		handler := middleware(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			Expect(RequestID(request.Context())).NotTo(BeEmpty())
			Expect(CorrelationID(request.Context())).To(Equal(RequestID(request.Context())))
			writer.WriteHeader(http.StatusCreated)
		}))
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", nil)
		response := httptest.NewRecorder()

		handler.ServeHTTP(response, request)

		Expect(response.Code).To(Equal(http.StatusCreated))
		Expect(response.Header().Get("X-Request-ID")).NotTo(BeEmpty())
		Expect(response.Header().Get("X-Correlation-ID")).To(Equal(response.Header().Get("X-Request-ID")))
		Expect(response.Header().Get("Traceparent")).NotTo(BeEmpty())
		Expect(logger.value("request_id")).To(Equal(response.Header().Get("X-Request-ID")))
		Expect(logger.value("correlation_id")).To(Equal(response.Header().Get("X-Correlation-ID")))
		Expect(logger.value("status")).To(Equal("201"))
		Expect(UserID(WithUserID(request.Context(), "0192f8a0-7b00-7000-8000-000000000001"))).To(Equal("0192f8a0-7b00-7000-8000-000000000001"))
	})

	It("handles the local Flutter Web CORS preflight", func() {
		previous := os.Getenv("APP_ENV")
		DeferCleanup(func() {
			Expect(os.Setenv("APP_ENV", previous)).To(Succeed())
		})
		Expect(os.Setenv("APP_ENV", "local")).To(Succeed())

		middleware, err := New(&fakeLifecycle{}, &captureLogger{})
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
		logger := &captureLogger{}
		middleware, err := New(&fakeLifecycle{}, logger)
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
		Expect(logger.value("lambda_request_id")).To(Equal("lambda-request"))
	})

	It("uses a safe default sample rate and preserves incoming trace context", func() {
		logger := &captureLogger{}
		middleware, err := New(&fakeLifecycle{}, logger)
		Expect(err).NotTo(HaveOccurred())
		request := httptest.NewRequest(http.MethodGet, "/health", nil)
		request.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
		response := httptest.NewRecorder()

		middleware(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			writer.WriteHeader(http.StatusInternalServerError)
		})).ServeHTTP(response, request)

		Expect(response.Header().Get("Traceparent")).To(MatchRegexp(`^00-4bf92f3577b34da6a3ce929d0e0e4736-[0-9a-f]{16}-01$`))
		Expect(logger.value("status")).To(Equal("500"))
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

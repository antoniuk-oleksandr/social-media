package observability

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/antoniuk-oleksandr/social-media/internal/platform/logging"
	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/smithy-go/middleware"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/fx"
)

type contextKey string

const (
	requestIDKey     contextKey = "social-media.request-id"
	correlationIDKey contextKey = "social-media.correlation-id"
	userIDKey        contextKey = "social-media.user-id"
)

type Middleware func(http.Handler) http.Handler

const localWebOrigin = "http://localhost:8081"

// DynamoDBAPIOption instruments every AWS SDK DynamoDB operation without
// coupling feature repositories to OpenTelemetry or the AWS middleware API.
func DynamoDBAPIOption() func(*middleware.Stack) error {
	return func(stack *middleware.Stack) error {
		return stack.Finalize.Add(newDynamoDBTracingMiddleware(), middleware.After)
	}
}

func newDynamoDBTracingMiddleware() middleware.FinalizeMiddleware {
	return middleware.FinalizeMiddlewareFunc("social-media-dynamodb-tracing", func(ctx context.Context, input middleware.FinalizeInput, next middleware.FinalizeHandler) (middleware.FinalizeOutput, middleware.Metadata, error) {
		operation := middleware.GetOperationName(ctx)
		if operation == "" {
			operation = "DynamoDB"
		}
		ctx, span := otel.Tracer("social-media/dynamodb").Start(ctx, "DynamoDB."+operation)
		output, metadata, err := next.HandleFinalize(ctx, input)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "DynamoDB operation failed")
		}
		span.End()
		return output, metadata, err
	})
}

func New(lifecycle fx.Lifecycle, logger logging.Logger) (Middleware, error) {
	provider, err := newTracerProvider()
	if err != nil {
		return nil, err
	}
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.TraceContext{})
	lifecycle.Append(fx.Hook{OnStop: provider.Shutdown})

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if request.Header.Get("Origin") == localWebOrigin && strings.EqualFold(os.Getenv("APP_ENV"), "local") {
				writer.Header().Set("Access-Control-Allow-Origin", localWebOrigin)
				writer.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Traceparent, X-Correlation-ID, X-Request-ID")
				writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
				writer.Header().Set("Access-Control-Expose-Headers", "Traceparent, X-Correlation-ID, X-Request-ID")
				writer.Header().Set("Access-Control-Max-Age", "300")
				if request.Method == http.MethodOptions {
					writer.WriteHeader(http.StatusNoContent)
					return
				}
			}
			started := time.Now()
			requestID := request.Header.Get("X-Request-ID")
			if requestID == "" {
				requestID = lambdaRequestID(request.Context())
			}
			if requestID == "" {
				requestID = uuid.NewString()
			}
			correlationID := request.Header.Get("X-Correlation-ID")
			if correlationID == "" {
				correlationID = requestID
			}

			ctx := otel.GetTextMapPropagator().Extract(request.Context(), propagation.HeaderCarrier(request.Header))
			ctx, span := otel.Tracer("social-media/http").Start(ctx, request.Method+" "+request.URL.Path,
				oteltrace.WithSpanKind(oteltrace.SpanKindServer),
			)
			defer span.End()
			ctx = context.WithValue(ctx, requestIDKey, requestID)
			ctx = context.WithValue(ctx, correlationIDKey, correlationID)
			request = request.WithContext(ctx)
			writer.Header().Set("X-Request-ID", requestID)
			writer.Header().Set("X-Correlation-ID", correlationID)
			otel.GetTextMapPropagator().Inject(ctx, propagation.HeaderCarrier(writer.Header()))

			response := &statusWriter{ResponseWriter: writer}
			next.ServeHTTP(response, request)
			status := response.status
			if status == 0 {
				status = http.StatusOK
			}
			fields := []logging.Field{
				logging.String("request_id", requestID),
				logging.String("correlation_id", correlationID),
				logging.String("lambda_request_id", lambdaRequestID(request.Context())),
				logging.String("method", request.Method),
				logging.String("route", request.URL.Path),
				logging.String("status", strconv.Itoa(status)),
				logging.String("duration", time.Since(started).String()),
			}
			if spanContext := oteltrace.SpanContextFromContext(ctx); spanContext.IsValid() {
				fields = append(fields,
					logging.String("trace_id", spanContext.TraceID().String()),
					logging.String("span_id", spanContext.SpanID().String()),
				)
			}
			if userID := UserID(ctx); userID != "" {
				fields = append(fields, logging.String("user_id", userID))
			}
			if status >= http.StatusInternalServerError {
				if logErr := logger.Error("request completed with server error", fields...); logErr != nil {
					return
				}
				return
			}
			if logErr := logger.Info("request completed", fields...); logErr != nil {
				return
			}
		})
	}, nil
}

func RequestID(ctx context.Context) string {
	value, _ := ctx.Value(requestIDKey).(string)
	return value
}

func CorrelationID(ctx context.Context) string {
	value, _ := ctx.Value(correlationIDKey).(string)
	return value
}

func WithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

func UserID(ctx context.Context) string {
	value, _ := ctx.Value(userIDKey).(string)
	return value
}

func newTracerProvider() (*trace.TracerProvider, error) {
	sampler := trace.ParentBased(trace.TraceIDRatioBased(sampleRate()))
	appResource, err := newResource()
	if err != nil {
		return nil, err
	}
	options := []trace.TracerProviderOption{
		trace.WithSampler(sampler),
		trace.WithResource(appResource),
	}
	endpoint := strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	if endpoint != "" && strings.EqualFold(os.Getenv("OTEL_SDK_DISABLED"), "false") {
		endpoint = strings.TrimPrefix(strings.TrimPrefix(endpoint, "http://"), "https://")
		exporter, err := otlptracehttp.New(context.Background(), otlptracehttp.WithEndpoint(endpoint), otlptracehttp.WithInsecure())
		if err != nil {
			return nil, fmt.Errorf("create OTLP trace exporter: %w", err)
		}
		if strings.EqualFold(os.Getenv("APP_ENV"), "local") {
			options = append(options, trace.WithSpanProcessor(trace.NewSimpleSpanProcessor(exporter)))
		} else {
			options = append(options, trace.WithBatcher(exporter))
		}
	}
	return trace.NewTracerProvider(options...), nil
}

func newResource() (*resource.Resource, error) {
	serviceName := strings.TrimSpace(os.Getenv("OTEL_SERVICE_NAME"))
	if serviceName == "" {
		return resource.Default(), nil
	}
	appResource, err := resource.Merge(
		resource.Default(),
		resource.NewSchemaless(attribute.String("service.name", serviceName)),
	)
	if err != nil {
		return nil, fmt.Errorf("create OTEL resource: %w", err)
	}
	return appResource, nil
}

func sampleRate() float64 {
	rate, err := strconv.ParseFloat(os.Getenv("OTEL_TRACES_SAMPLER_ARG"), 64)
	if err != nil || rate < 0 || rate > 1 {
		return 0.1
	}
	return rate
}

func lambdaRequestID(ctx context.Context) string {
	value, ok := lambdacontext.FromContext(ctx)
	if !ok || value == nil {
		return ""
	}
	return value.AwsRequestID
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(data)
}

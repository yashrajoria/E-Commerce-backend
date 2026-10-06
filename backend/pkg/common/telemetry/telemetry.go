// Package telemetry provides optional OpenTelemetry tracing for all Go services.
//
// # Local Development
//
// To see traces locally, run Jaeger all-in-one:
//
//	docker run -d --name jaeger \
//	  -p 16686:16686 \
//	  -p 4318:4318 \
//	  jaegertracing/all-in-one:1.57
//
// Then set:
//
//	OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318
//
// If OTEL_EXPORTER_OTLP_ENDPOINT is unset, telemetry is disabled (no-op).
// Services will still start and function normally without any tracing overhead.
//
// # Production
//
// Set OTEL_EXPORTER_OTLP_ENDPOINT to your OTLP collector endpoint.
// Set DEPLOY_ENV or APP_ENV to the deployment stage (default: "local").
package telemetry

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

var (
	tracer   trace.Tracer
	enabled  bool
	provider *sdktrace.TracerProvider
	exporter *otlptrace.Exporter
)

// Init initializes OpenTelemetry tracing. If OTEL_EXPORTER_OTLP_ENDPOINT is unset,
// it returns a no-op configuration and logs once that telemetry is disabled.
func Init(ctx context.Context, serviceName string) (shutdown func()) {
	endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	if endpoint == "" || (strings.Contains(endpoint, "jaeger") && (os.Getenv("RENDER") != "" || os.Getenv("PORT") != "")) {
		log.Printf("[telemetry] OTEL_EXPORTER_OTLP_ENDPOINT disabled or jaeger host unresolvable — telemetry disabled for %s", serviceName)
		tracer = noop.NewTracerProvider().Tracer(serviceName)
		enabled = false
		return func() {}
	}

	// OTLP/HTTP endpoint must include scheme; default insecure for local Jaeger/collector.
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		endpoint = "http://" + endpoint
	}
	// WithEndpointURL does NOT append the OTLP path automatically — without
	// this, exports POST to "/" and every collector returns 404 (silently
	// swallowed by BatchSpanProcessor).
	if !strings.HasSuffix(endpoint, "/v1/traces") {
		endpoint = strings.TrimRight(endpoint, "/") + "/v1/traces"
	}

	env := os.Getenv("DEPLOY_ENV")
	if env == "" {
		env = os.Getenv("APP_ENV")
	}
	if env == "" {
		env = "local"
	}

	var err error
	exporter, err = otlptracehttp.New(ctx,
		otlptracehttp.WithEndpointURL(endpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		log.Printf("[telemetry] failed to create OTLP exporter for %s: %v (telemetry disabled)", serviceName, err)
		tracer = noop.NewTracerProvider().Tracer(serviceName)
		enabled = false
		return func() {}
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.DeploymentEnvironmentName(env),
		),
	)
	if err != nil {
		log.Printf("[telemetry] failed to create resource for %s: %v", serviceName, err)
	}

	provider = sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)

	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	tracer = provider.Tracer(serviceName)
	enabled = true

	log.Printf("[telemetry] enabled for %s (endpoint=%s, env=%s)", serviceName, endpoint, env)

	return func() {
		if provider != nil {
			_ = provider.Shutdown(ctx)
		}
		if exporter != nil {
			_ = exporter.Shutdown(ctx)
		}
	}
}

// Tracer returns the global tracer for manual span creation.
// Returns a no-op tracer if telemetry is disabled.
func Tracer() trace.Tracer {
	return tracer
}

// IsEnabled returns true if telemetry was successfully initialized.
func IsEnabled() bool {
	return enabled
}

// GinMiddleware returns a Gin middleware that:
//   - Extracts incoming W3C traceparent headers
//   - Starts a server span with http.route as the span name (or method+path as fallback)
//   - Sets http.response.status_code attribute
//   - Injects the active trace context into response headers so clients can correlate
//
// If telemetry is disabled, this is a no-op middleware.
func GinMiddleware(serviceName string) gin.HandlerFunc {
	if !enabled {
		return func(c *gin.Context) { c.Next() }
	}
	return otelgin.Middleware(serviceName,
		otelgin.WithSpanNameFormatter(func(c *gin.Context) string {
			// Prefer route pattern (e.g., /users/:id) to avoid cardinality explosion.
			// gin's FullPath() is empty during handler execution, so we fall back to path.
			route := c.FullPath()
			if route == "" {
				route = c.Request.URL.Path
			}
			return c.Request.Method + " " + route
		}),
	)
}

// HTTPTransport wraps an http.RoundTripper to inject trace context into outbound
// requests. The wrapper is always installed and checks `enabled` at RoundTrip
// time — critical because package-level http.Client vars (e.g. the gateway
// forwarder) initialize before main() calls Init().
func HTTPTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return &tracingTransport{base: base}
}

type tracingTransport struct {
	base http.RoundTripper
}

func (t *tracingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if enabled && trace.SpanFromContext(req.Context()).SpanContext().IsValid() {
		otel.GetTextMapPropagator().Inject(req.Context(), propagation.HeaderCarrier(req.Header))
	}
	return t.base.RoundTrip(req)
}

// StartSQSConsumerSpan starts a span for SQS message processing.
// The returned context carries the span. Call the returned function to end the span.
// If telemetry is disabled, this returns a no-op span.
func StartSQSConsumerSpan(ctx context.Context, queueURL, messageID string) (context.Context, func()) {
	if !enabled {
		return ctx, func() {}
	}
	ctx, span := tracer.Start(ctx, "sqs.consume",
		trace.WithAttributes(
			attribute.String("messaging.system", "aws.sqs"),
			attribute.String("messaging.destination.name", queueURL),
			attribute.String("messaging.message.id", messageID),
		),
		trace.WithSpanKind(trace.SpanKindConsumer),
	)
	return ctx, func() { span.End() }
}

// StartSNSSendSpan starts a span for SNS publish operations.
// The returned context carries the span. Call the returned function to end the span.
// If telemetry is disabled, this returns a no-op span.
func StartSNSSendSpan(ctx context.Context, topicARN string) (context.Context, func()) {
	if !enabled {
		return ctx, func() {}
	}
	ctx, span := tracer.Start(ctx, "sns.publish",
		trace.WithAttributes(
			attribute.String("messaging.system", "aws.sns"),
			attribute.String("messaging.destination.name", topicARN),
		),
		trace.WithSpanKind(trace.SpanKindProducer),
	)
	return ctx, func() { span.End() }
}

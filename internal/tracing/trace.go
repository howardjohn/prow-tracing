package tracing

import (
	"context"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
	"os"
	"time"
)

type Tracer struct {
	tracer trace.Tracer
	tp     *tracesdk.TracerProvider
}

func New() (*Tracer, error) {
	otel.Tracer("prowjob")
	_ = jaeger.New
	_ = os.Stdout
	_ = stdouttrace.New
	exp, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint("http://localhost:14268/api/traces")))
	//exp, err := stdouttrace.New(stdouttrace.WithWriter(os.Stdout), stdouttrace.WithPrettyPrint())
	if err != nil {
		return nil, err
	}
	tp := tracesdk.NewTracerProvider(
		tracesdk.WithSampler(tracesdk.AlwaysSample()),
		// Always be sure to batch in production.
		tracesdk.WithBatcher(exp),
		// Record information about this application in a Resource.
		tracesdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("prowjob"),
		)),
	)
	tracer := tp.Tracer("prowjob")
	//gctx, gspan := tracer.Start(context.Background(), "job")
	return &Tracer{
		tracer: tracer,
		tp:     tp,
	}, nil
}

func (t *Tracer) Shutdown() {
	t.tp.ForceFlush(context.Background())
	t.tp.Shutdown(context.Background())
}

func (t *Tracer) Record(name string, start, end time.Time) Context {
	return Context{t, context.Background()}.Record(name, start, end)
}

type Context struct {
	tracer *Tracer
	ctx    context.Context
}

func (c Context) Record(name string, start, end time.Time) Context {
	ctx, span := c.tracer.tracer.Start(c.ctx, name, trace.WithTimestamp(start))
	span.AddEvent()
	// TODO: do we need to do this in shutdown so we can mutate the span still?
	span.End(trace.WithTimestamp(end))
	return Context{c.tracer, ctx}
}

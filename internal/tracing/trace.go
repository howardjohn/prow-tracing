package tracing

import (
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"

	"github.com/howardjohn/prow-tracing/internal/model"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"go.opentelemetry.io/otel/trace"
)

func exporter() (tracesdk.TracerProviderOption, error) {
	proto := os.Getenv("OTEL_EXPORTER_OTLP_TRACES_PROTOCOL")
	if proto == "" {
		proto = os.Getenv("OTEL_EXPORTER_OTLP_PROTOCOL")
	}
	if proto == "" {
		proto = "grpc"
	}

	var c otlptrace.Client

	switch proto {
	case "grpc":
		c = otlptracegrpc.NewClient()
		log.Printf("using gRPC")
	case "http/protobuf":
		c = otlptracehttp.NewClient()
		log.Printf("using HTTP")
	// case "http/json": // unsupported by library
	default:
		return nil, fmt.Errorf("unsupported otlp protocol %v", proto)
	}
	traceExporter, err := otlptrace.New(context.Background(), c)
	if err != nil {
		return nil, fmt.Errorf("failed to create trace exporter: %w", err)
	}
	return tracesdk.WithSpanProcessor(tracesdk.NewBatchSpanProcessor(traceExporter)), nil
}

type idGenerator struct {
	sync.Mutex
	randSource *rand.Rand
	pj         model.ProwJob
}

var _ tracesdk.IDGenerator = &idGenerator{}

// NewSpanID returns a non-zero span ID from a randomly-chosen sequence.
func (gen *idGenerator) NewSpanID(ctx context.Context, traceID trace.TraceID) trace.SpanID {
	gen.Lock()
	defer gen.Unlock()
	sid := trace.SpanID{}
	_, _ = gen.randSource.Read(sid[:])
	return sid
}

// NewIDs returns a non-zero trace ID and a non-zero span ID from a
// randomly-chosen sequence.
func (gen *idGenerator) NewIDs(ctx context.Context) (trace.TraceID, trace.SpanID) {
	gen.Lock()
	defer gen.Unlock()
	u, _ := Parse(gen.pj.Labels["prow.k8s.io/id"])
	tid := trace.TraceID(u)
	sid := trace.SpanID(u[0:8])
	return tid, sid
}

func newIdGenerator(pj model.ProwJob) tracesdk.IDGenerator {
	gen := &idGenerator{pj: pj}
	var rngSeed int64
	_ = binary.Read(crand.Reader, binary.LittleEndian, &rngSeed)
	gen.randSource = rand.New(rand.NewSource(rngSeed))
	return gen
}

func NewAction(uid string) (Context, func(), error) {
	otel.Tracer("prowjob")
	exp, err := exporter()
	if err != nil {
		return Context{}, func() {}, err
	}

	tp := tracesdk.NewTracerProvider(
		tracesdk.WithSampler(tracesdk.AlwaysSample()),
		exp,
		tracesdk.WithResource(resource.NewWithAttributes(semconv.SchemaURL, semconv.ServiceName("test"))),
	)
	tracer := tp.Tracer("prowjob-trace")
	ctx := context.Background()
	p := propagation.TraceContext{}
	u, _ := Parse(uid)
	parent := fmt.Sprintf("%02x-%032x-%016x-%02x", 1, u, u[0:8], 0)
	ctx = p.Extract(ctx, propagation.MapCarrier{"traceparent": parent})
	shutdown := func() {
		log.Printf("flush %v\n", tp.ForceFlush(context.Background()))
		log.Printf("shutdown %v\n", tp.Shutdown(context.Background()))
	}

	c := Context{tracer: tracer, ctx: ctx}
	return c, shutdown, nil
}

func NewRoot(pj model.ProwJob) (Context, func(), error) {
	otel.Tracer("prowjob")
	exp, err := exporter()
	if err != nil {
		return Context{}, func() {}, err
	}

	attrs := attrFromProwjob(pj)
	attrs = append(attrs, semconv.ServiceName("prowjob"))
	tp := tracesdk.NewTracerProvider(
		tracesdk.WithSampler(tracesdk.AlwaysSample()),
		exp,
		tracesdk.WithResource(resource.NewWithAttributes(semconv.SchemaURL, attrs...)),
		tracesdk.WithIDGenerator(newIdGenerator(pj)),
	)
	tracer := tp.Tracer("prowjob-trace")
	ctx := context.Background()
	shutdown := func() {
		log.Printf("flush %v\n", tp.ForceFlush(context.Background()))
		log.Printf("shutdown %v\n", tp.Shutdown(context.Background()))
	}

	c := Context{tracer: tracer, ctx: ctx}
	return c, shutdown, nil
}

func attrFromProwjob(pj model.ProwJob) []attribute.KeyValue {
	res := []attribute.KeyValue{}
	for k, v := range pj.Labels {
		if strings.Contains(k, "prow.k8s.io") {
			res = append(res, attribute.String(k, v))
		}
	}
	return res
}

type Context struct {
	tracer trace.Tracer
	ctx    context.Context
}

func (c Context) Record(name string, start, end time.Time) Context {
	return c.Recording(name, start, end).End()
}

func (c Context) Recording(name string, start, end time.Time) Recording {
	ctx, span := c.tracer.Start(c.ctx, name, trace.WithTimestamp(start))

	return Recording{
		tracer: c.tracer,
		ctx:    ctx,
		end:    end,
		span:   span,
	}
}

type Recording struct {
	tracer trace.Tracer
	ctx    context.Context
	end    time.Time
	span   trace.Span
}

func (c Recording) Event(msg string, t time.Time, attrs ...attribute.KeyValue) {
	c.span.AddEvent(msg, trace.WithTimestamp(t), trace.WithAttributes(attrs...))
}

func (c Recording) End() Context {
	log.Printf("span %v ending", c.span.SpanContext().SpanID())
	c.span.End(trace.WithTimestamp(c.end))
	return Context{tracer: c.tracer, ctx: c.ctx}
}

type UUID [16]byte

func Parse(s string) (UUID, error) {
	var uuid UUID
	if len(s) != 36 {
		if len(s) != 36+9 {
			return uuid, fmt.Errorf("invalid UUID length: %d", len(s))
		}
		if strings.ToLower(s[:9]) != "urn:uuid:" {
			return uuid, fmt.Errorf("invalid urn prefix: %q", s[:9])
		}
		s = s[9:]
	}
	if s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		return uuid, errors.New("invalid UUID format")
	}
	for i, x := range [16]int{
		0, 2, 4, 6,
		9, 11,
		14, 16,
		19, 21,
		24, 26, 28, 30, 32, 34} {
		v, ok := xtob(s[x], s[x+1])
		if !ok {
			return uuid, errors.New("invalid UUID format")
		}
		uuid[i] = v
	}
	return uuid, nil
}

// xvalues returns the value of a byte as a hexadecimal digit or 255.
var xvalues = [256]byte{
	255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255,
	255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255,
	255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255,
	0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 255, 255, 255, 255, 255, 255,
	255, 10, 11, 12, 13, 14, 15, 255, 255, 255, 255, 255, 255, 255, 255, 255,
	255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255,
	255, 10, 11, 12, 13, 14, 15, 255, 255, 255, 255, 255, 255, 255, 255, 255,
	255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255,
	255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255,
	255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255,
	255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255,
	255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255,
	255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255,
	255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255,
	255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255,
	255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255, 255,
}

// xtob converts hex characters x1 and x2 into a byte.
func xtob(x1, x2 byte) (byte, bool) {
	b1 := xvalues[x1]
	b2 := xvalues[x2]
	return (b1 << 4) | b2, b1 != 255 && b2 != 255
}

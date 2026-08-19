package telemetry

import (
	"context"
	"database/sql"
	"net"
	"strings"
	"time"

	redis "github.com/redis/go-redis/v9"
	"github.com/uptrace/bun"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type BunQueryHook struct {
	runtime *Runtime
	metrics *Registry
}

func NewBunQueryHook(runtime *Runtime, metrics *Registry) *BunQueryHook {
	return &BunQueryHook{runtime: runtime, metrics: metrics}
}

func (h *BunQueryHook) BeforeQuery(ctx context.Context, event *bun.QueryEvent) context.Context {
	op := strings.ToUpper(event.Operation())
	ctx, _ = h.runtime.Tracer("wego/postgres").Start(ctx, "postgres "+op,
		trace.WithSpanKind(trace.SpanKindClient),
		trace.WithAttributes(attribute.String("db.system.name", "postgresql"), attribute.String("db.operation.name", op)),
	)
	return ctx
}

func (h *BunQueryHook) AfterQuery(ctx context.Context, event *bun.QueryEvent) {
	span := trace.SpanFromContext(ctx)
	defer span.End()
	result := "success"
	if event.Err != nil && event.Err != sql.ErrNoRows {
		result = "error"
		span.RecordError(event.Err)
		span.SetStatus(codes.Error, event.Err.Error())
	}
	labels := map[string]string{"operation": strings.ToUpper(event.Operation()), "result": result}
	h.metrics.Inc("wego_postgres_queries_total", labels)
	h.metrics.Observe("wego_postgres_query_duration_seconds", labels, time.Since(event.StartTime).Seconds())
}

type RedisHook struct {
	runtime *Runtime
	metrics *Registry
}

func NewRedisHook(runtime *Runtime, metrics *Registry) *RedisHook {
	return &RedisHook{runtime: runtime, metrics: metrics}
}

func (h *RedisHook) DialHook(next redis.DialHook) redis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		ctx, span := h.runtime.Tracer("wego/redis").Start(ctx, "redis connect", trace.WithSpanKind(trace.SpanKindClient))
		defer span.End()
		conn, err := next(ctx, network, addr)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		return conn, err
	}
}

func (h *RedisHook) ProcessHook(next redis.ProcessHook) redis.ProcessHook {
	return func(ctx context.Context, cmd redis.Cmder) error {
		op := strings.ToUpper(cmd.FullName())
		ctx, span := h.runtime.Tracer("wego/redis").Start(ctx, "redis "+op,
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(attribute.String("db.system.name", "redis"), attribute.String("db.operation.name", op)),
		)
		start := time.Now()
		err := next(ctx, cmd)
		span.End()
		h.record(op, start, err)
		return err
	}
}

func (h *RedisHook) ProcessPipelineHook(next redis.ProcessPipelineHook) redis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []redis.Cmder) error {
		ctx, span := h.runtime.Tracer("wego/redis").Start(ctx, "redis pipeline",
			trace.WithSpanKind(trace.SpanKindClient),
			trace.WithAttributes(attribute.Int("db.operation.batch.size", len(cmds))),
		)
		start := time.Now()
		err := next(ctx, cmds)
		span.End()
		h.record("PIPELINE", start, err)
		return err
	}
}

func (h *RedisHook) record(operation string, start time.Time, err error) {
	result := "success"
	if err != nil && err != redis.Nil {
		result = "error"
	}
	labels := map[string]string{"operation": operation, "result": result}
	h.metrics.Inc("wego_redis_commands_total", labels)
	h.metrics.Observe("wego_redis_command_duration_seconds", labels, time.Since(start).Seconds())
}

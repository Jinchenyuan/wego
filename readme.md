### Project Overview
This is a microservices project based on **go-micro**, It is suitable for lightweight mobile apps or small game backends.

### Tech Stack

#### Framework
- API gateway / HTTP handler: **Gin**
- Microservices framework: **go-micro**

#### Drivers
- Database: **pgx** (PostgreSQL driver)
- Cache: **go-redis** (Redis client)

#### Middleware & Components
- Cache: **Redis**
- Database: **PostgreSQL**
- Service discovery & configuration: **etcd**

## Reminder MVP

The framework now includes a minimal reminder worker built on PostgreSQL and the Mesa lifecycle.

### Included capabilities

- Store reminder records in PostgreSQL
- Poll due reminders in the background
- Deliver reminders through a user-supplied notifier
- Retry failed deliveries with backoff
- Recover stale processing reminders after restart
- Cancel or reschedule reminders by key

### Basic usage

```go
mesa, err := wego.New(
	wego.WithDSN(dsn),
)
if err != nil {
	panic(err)
}

svc := reminder.NewService(
	mesa.DB,
	reminder.NotifierFunc(func(ctx context.Context, r *reminder.Reminder) error {
		// send email, push, webhook, etc.
		return nil
	}),
)

// Run versioned migrations before starting Mesa (normally in deployment tooling).
if err := reminder.Migrate(context.Background(), mesa.DB); err != nil {
	panic(err)
}

if err := mesa.RegisterComponent(svc); err != nil {
	panic(err)
}

reminderSvc, ok := wego.GetComponentAs[*reminder.Service](mesa, svc.Name())
if !ok {
	panic("reminder component not found")
}

_, err := reminderSvc.Create(context.Background(), reminder.CreateParams{
	Key:        "order-123-pay-deadline",
	UserID:     "42",
	Channel:    "in_app",
	Payload:    `{"title":"Payment due soon"}`,
	ScheduleAt: time.Now().Add(10 * time.Minute),
})
```

### Querying and state errors

Reminder schema changes are explicit: call `reminder.Migrate(ctx, db)` before the worker starts. `Service.Start` never creates or alters tables.

```go
item, err := reminderSvc.GetByKey(ctx, "order-123-pay-deadline")
if errors.Is(err, reminder.ErrNotFound) {
	// handle a missing reminder
}

page, err := reminderSvc.List(ctx, reminder.ListParams{
	UserID:   "42",
	Statuses: []reminder.Status{reminder.StatusPending},
	Limit:    50,
})
nextPage, err := reminderSvc.List(ctx, reminder.ListParams{
	UserID: "42",
	Limit:  50,
	Cursor: page.NextCursor,
})

_ = item
_ = nextPage
```

`Create` returns `ErrAlreadyExists` for duplicate keys. Cancel and reschedule return `ErrNotFound` for unknown keys and `ErrStateConflict` when the current state does not allow the operation. Canceling an already canceled reminder succeeds.

### Dispatcher notifier

When reminder scenes grow, you can register handlers by `channel`, by payload `type`, or by an exact `channel + type` route.

```go
dispatcher := reminder.NewDispatcherNotifier()

_ = dispatcher.RegisterChannel("in_app", reminder.NotifierFunc(func(ctx context.Context, r *reminder.Reminder) error {
	// generic in-app delivery
	return nil
}))

_ = dispatcher.RegisterType("payment_due", reminder.NotifierFunc(func(ctx context.Context, r *reminder.Reminder) error {
	// shared payment_due template logic
	return nil
}))

_ = dispatcher.Register("sms", "payment_due", reminder.NotifierFunc(func(ctx context.Context, r *reminder.Reminder) error {
	// exact route for sms + payment_due
	return nil
}))

_ = dispatcher.SetFallback(reminder.NotifierFunc(func(ctx context.Context, r *reminder.Reminder) error {
	// last-resort handler
	return nil
}))

mesa, err := wego.New(
	wego.WithDSN(dsn),
)
if err != nil {
	panic(err)
}

svc := reminder.NewService(mesa.DB, dispatcher)

if err := mesa.RegisterComponent(svc); err != nil {
	panic(err)
}

component, ok := mesa.GetComponent(svc.Name())
if !ok {
	panic("component not found")
}

reminderSvc := component.(*reminder.Service)

_, _ = reminderSvc.Create(ctx, reminder.CreateParams{
	Key:        "order-123-pay-deadline",
	UserID:     "42",
	Channel:    "sms",
	Payload:    `{"type":"payment_due","title":"Payment due soon"}`,
	ScheduleAt: time.Now().Add(10 * time.Minute),
})
```

Dispatch priority is:

- exact `channel + type`
- payload `type`
- `channel`
- fallback

### Component registry

Mesa now acts as a generic component registry.

```go
if err := mesa.RegisterComponent(svc); err != nil {
	panic(err)
}

component, ok := mesa.GetComponent("reminder")
if !ok {
	panic("component not found")
}

reminderSvc, ok := wego.GetComponentAs[*reminder.Service](mesa, "reminder")
if !ok {
	panic("unexpected component type")
}

_ = component
_ = reminderSvc
```

### Default component configuration

`WithComponents(...)` lets Mesa auto-register built-in components in one place.

```go
mesa, err := wego.New(
	wego.WithDSN(dsn),
	wego.WithRedisConfig(wego.RedisConfig{
		Addr: "127.0.0.1:6379",
	}),
	wego.WithComponents(wego.ComponentsConfig{
		Reminder: wego.ReminderComponentConfig{
			Enabled:  true,
			Notifier: reminder.NotifierFunc(func(ctx context.Context, r *reminder.Reminder) error {
				return nil
			}),
			PollInterval: 2 * time.Second,
		},
		PubSub: wego.PubSubComponentConfig{
			Enabled:      true,
			StreamPrefix: "wego:events",
		},
	}),
)
if err != nil {
	panic(err)
}

reminderSvc := mesa.MustGetReminder()
pubsubSvc := mesa.MustGetPubSub()

_ = reminderSvc
_ = pubsubSvc
```

### Pub/Sub component

`pubsub` provides a Redis Streams based component for cross-instance publish and subscribe with consumer groups.

### Included capabilities

- Publish messages to a topic backed by Redis Streams
- Register subscriptions before Mesa runtime starts
- Competing consumers within one consumer group
- Fan-out delivery through different consumer groups
- Retry failed deliveries with configurable backoff
- Ack on success and dead-letter on max delivery attempts

### Basic usage

```go
mesa, err := wego.New(
	wego.WithDSN(dsn),
	wego.WithRedisConfig(wego.RedisConfig{
		Addr: "127.0.0.1:6379",
	}),
	wego.WithComponents(wego.ComponentsConfig{
		PubSub: wego.PubSubComponentConfig{
			Enabled: true,
		},
	}),
)
if err != nil {
	panic(err)
}

pubsubSvc := mesa.MustGetPubSub()

pubsubSvc.MustSubscribe(pubsub.Subscription{
	Topic: "order.created",
	Group: "billing",
	Handler: pubsub.HandlerFunc(func(ctx context.Context, delivery *pubsub.Delivery) error {
		fmt.Println("received:", delivery.Message.Type, string(delivery.Message.Data))
		return nil
	}),
})

_, err := pubsubSvc.Publish(context.Background(), "order.created", pubsub.Message{
	Key:  "order-123",
	Type: "order.created",
	Data: []byte(`{"id":"123"}`),
})
if err != nil {
	panic(err)
}
```

Runtime rules are:

- subscriptions must be registered before `mesa.Run(ctx)`
- handlers must be idempotent because delivery is at least once
- one `topic + group` maps to one handler within a process
- use different consumer groups when the same topic needs fan-out
- handler failures are re-published through an internal retry queue using the configured backoff policy
- retry queues are isolated by `topic + group`; scheduling a retry and acknowledging the source entry are atomic on Redis
- auto-registration only happens when `wego.WithComponents(...)` or `wego.WithPubSub(...)` is provided and Redis is configured

Pub/Sub can also be configured with framework-level fields such as `Name`, `RetryPolicy`, `StreamPrefix`, `StreamMaxLen`, `StreamBlock`, `StreamReadCount`, and dead-letter options through `wego.PubSubComponentConfig`.

### Pub/Sub integration tests

The Redis-backed integration tests for `pubsub` are opt-in and use the `integration` build tag.

Required environment variables:

- `WEGO_REDIS_TEST_ADDR`: Redis address such as `127.0.0.1:6379`

Optional environment variables:

- `WEGO_REDIS_TEST_PASSWORD`: Redis password
- `WEGO_REDIS_TEST_DB`: Redis database index, defaults to `15`

Run the default unit tests:

```bash
go test ./pubsub
```

Run the Redis integration tests:

```bash
WEGO_REDIS_TEST_ADDR=127.0.0.1:6379 go test -tags=integration ./pubsub
```

Run the full repository test suite without integration tests:

```bash
go test ./...
```

If `WEGO_REDIS_TEST_ADDR` is not set or Redis is unreachable, the integration tests are skipped.

### Reminder integration tests

PostgreSQL-backed Reminder tests are opt-in. Each test creates and removes an isolated schema and applies the production migration.

```bash
WEGO_POSTGRES_TEST_DSN='postgres://user:password@127.0.0.1:5432/land_contract?sslmode=disable' go test -tags=integration ./reminder
```

### TCP transport

`transport/tcp` provides a generic TCP server implementation that matches the existing `transport.Server` lifecycle.

```go
tcpServer := tcp.NewTCPServer(
	tcp.WithHost(net.ParseIP("0.0.0.0")),
	tcp.WithPort(9000),
	tcp.WithHandler(func(ctx context.Context, conn net.Conn) {
		defer conn.Close()

		buf := make([]byte, 1024)
		n, err := conn.Read(buf)
		if err != nil {
			return
		}

		_, _ = conn.Write(buf[:n])
	}),
)

mesa, err := wego.New(
	wego.WithDSN(dsn),
	wego.WithServers(tcpServer),
)
if err != nil {
	panic(err)
}
```

### Installation (main)
```
go get github.com/Jinchenyuan/wego@main
```

### Runtime and health checks

`New` only initializes configured dependencies and returns connection or configuration errors. `Run` accepts a context, and `Shutdown` is safe to call more than once.

When the default HTTP server is enabled with `WithHttpPort`, it exposes `GET /livez` and `GET /readyz`. The same status is available through `mesa.Liveness()` and `mesa.Readiness(ctx)`.

```go
mesa, err := wego.New(wego.WithHttpPort(8080))
if err != nil {
	return err
}

ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
defer stop()
return mesa.Run(ctx)
```

### Validated configuration

`wego.Config` is the typed production configuration surface. Applications remain
responsible for loading environment variables, files, and secrets, then call
`Validate` or `Options` before creating Mesa:

```go
cfg := wego.Config{
	Profile: wego.Profile{Name: "orders"},
	HTTP:    wego.HTTPConfig{Port: 8080},
	Redis:   wego.RedisConfig{Addr: "127.0.0.1:6379"},
}
opts, err := cfg.Options()
if err != nil {
	return err
}
mesa, err := wego.New(opts...)
```

Configuration errors include the invalid field path and may be inspected with
`errors.As(err, *wego.ConfigError)`.

### Pluggable authentication

`middleware.Authenticate` accepts a context-aware `Authenticator`. Successful
authentication stores a `Principal` in the request context for handlers to read
with `middleware.PrincipalFromContext`. The existing `AuthMiddleware` remains
available for header ID plus cached-token schemes.

### Production baseline

The default HTTP transport uses bounded header, request, response, idle, body,
and header sizes. Override them without breaking the legacy `WithHttpPort` API:

```go
m, err := wego.New(wego.WithHTTPConfig(wego.HTTPConfig{
	Port:              8080,
	ReadHeaderTimeout: 3 * time.Second,
	ReadTimeout:       10 * time.Second,
	WriteTimeout:      20 * time.Second,
	IdleTimeout:       60 * time.Second,
	MaxHeaderBytes:    1 << 20,
	MaxBodyBytes:      8 << 20,
	RequestTimeout:    15 * time.Second,
}))
```

`/livez`, `/readyz`, and `/metrics` are exposed by the default server. Logs are
newline-delimited JSON. Redis TLS can be configured with
`RedisConfig.TLSConfig`; TLS termination for public HTTP traffic is expected at
the Kubernetes Ingress. Reusable CORS, security-header, and per-instance rate
limit middleware is available in `middleware`.

### Deployable HTTP example

`examples/http-server` is a minimal application that uses the Mesa lifecycle,
registers `GET /hello`, and includes Docker and Helm deployment assets. Run it
locally from the repository root:

```sh
go run ./examples/http-server
curl http://127.0.0.1:8080/hello
```

The default server also exposes `/livez`, `/readyz`, and `/metrics`. Override
the port with `WEGO_HTTP_PORT`.

Build the image with the repository root as the Docker context:

```sh
docker build -f examples/http-server/Dockerfile -t wego-http-server-example .
docker run --rm -p 8080:8080 wego-http-server-example
```

Install the example chart after publishing the image to a registry:

```sh
helm upgrade --install wego-http-server \
  examples/http-server/deploy/helm/http-server \
  --set image.repository=registry.example.com/wego-http-server-example \
  --set image.tag=1.0.0
```

### Usage
Import packages to your .go files.

```go
"github.com/Jinchenyuan/wego"
"github.com/Jinchenyuan/wego/logger"
"github.com/Jinchenyuan/wego/middleware"
"github.com/Jinchenyuan/wego/transport"
"github.com/Jinchenyuan/wego/transport/micro"
```
Run a mesa instance with your configuration.

```go 
// read config from file
cfg, err := config.Read("config.toml")
if err != nil {
	fmt.Printf("failed to read config: %v\n", err)
	return
}
m, err := wego.New(
	wego.WithEtcdConfig(clientv3.Config{
		Endpoints:   cfg.Etcd.Endpoints,
		DialTimeout: 5 * time.Second,
		Username:    cfg.Etcd.User,
		Password:    cfg.Etcd.Password,
	}),
	wego.WithHttpPort(cfg.Http.Port),
	wego.WithDSN(cfg.PostgreSQL.DSN),
	wego.WithLogLevel(logger.ParseLevel(cfg.Log.Level)),
	wego.WithRedisConfig(wego.RedisConfig{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	}),
	wego.WithProfile(wego.Profile{
		Name: cfg.Profile.Name,
	}),
)
if err != nil {
	fmt.Printf("failed to initialize mesa: %v\n", err)
	return
}

ginhandler.SetAuthMiddleware(middleware.AuthMiddleware("account", func(id string) string {
	cacheToken, err := m.Redis.Get(context.Background(), fmt.Sprintf("token:%s", id)).Result()
	if err != nil {
		return ""
	}
	return cacheToken
}, cfg.Http.ExcludeAuthPaths...))

ginhandler.Registry()

ms := m.GetServerByType(transport.MICRO_SERVER).(*micro.Service)
ms.NewServiceClients(serviceclient.Registry)

if err := m.Run(context.Background()); err != nil {
	fmt.Printf("failed to run mesa: %v\n", err)
}
```

example project see: <https://github.com/Jinchenyuan/weserver>

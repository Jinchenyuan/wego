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
mesa := wego.New(
	wego.WithDSN(dsn),
)

svc := reminder.NewService(
	mesa.DB,
	reminder.NotifierFunc(func(ctx context.Context, r *reminder.Reminder) error {
		// send email, push, webhook, etc.
		return nil
	}),
)

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

mesa := wego.New(
	wego.WithDSN(dsn),
)

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

mesa := wego.New(
	wego.WithDSN(dsn),
	wego.WithServers(tcpServer),
)
```

### Installation (main)
```
go get github.com/Jinchenyuan/wego@main
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
m := wego.New(
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

if err := m.Run(); err != nil {
	fmt.Printf("failed to run mesa: %v\n", err)
}
```

example project see: <https://github.com/Jinchenyuan/weserver>

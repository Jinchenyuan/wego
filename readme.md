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
```bash
```

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


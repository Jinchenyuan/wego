package reminder

import "context"

type Notifier interface {
	Notify(context.Context, *Reminder) error
}

type NotifierFunc func(context.Context, *Reminder) error

func (f NotifierFunc) Notify(ctx context.Context, reminder *Reminder) error {
	return f(ctx, reminder)
}

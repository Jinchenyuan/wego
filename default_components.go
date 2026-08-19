package wego

import (
	"github.com/Jinchenyuan/wego/pubsub"
	"github.com/Jinchenyuan/wego/reminder"
)

type defaultComponentRegistrar struct {
	name     string
	enabled  func(options) bool
	register func(*Mesa) error
}

var defaultComponentRegistrars = []defaultComponentRegistrar{
	{
		name: "pubsub",
		enabled: func(opts options) bool {
			return opts.components.PubSub.Enabled
		},
		register: func(m *Mesa) error {
			if m.Redis == nil {
				return pubsub.ErrRedisNotConfigured
			}
			opts := append(m.opts.components.PubSub.buildOptions(), pubsub.WithTelemetry(m.Telemetry, m.Metrics))
			service := pubsub.NewService(m.Redis, opts...)
			return m.RegisterComponent(service)
		},
	},
	{
		name: "reminder",
		enabled: func(opts options) bool {
			return opts.components.Reminder.Enabled
		},
		register: func(m *Mesa) error {
			if m.DB == nil {
				return ErrReminderDBNotConfigured
			}
			if m.opts.components.Reminder.Notifier == nil {
				return ErrReminderNotifierNotConfigured
			}
			opts := append(m.opts.components.Reminder.buildOptions(), reminder.WithTelemetry(m.Telemetry, m.Metrics))
			service := reminder.NewService(
				m.DB,
				m.opts.components.Reminder.Notifier,
				opts...,
			)
			return m.RegisterComponent(service)
		},
	},
}

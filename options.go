package wego

import (
	"time"

	"github.com/Jinchenyuan/wego/logger"
	"github.com/Jinchenyuan/wego/pubsub"
	"github.com/Jinchenyuan/wego/reminder"
	"github.com/Jinchenyuan/wego/transport"
	"github.com/Jinchenyuan/wego/transport/micro"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type Options func(o *options)

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type PubSubComponentConfig struct {
	Enabled            bool
	Name               string
	RetryPolicy        *pubsub.RetryPolicy
	StreamPrefix       string
	StreamMaxLen       int64
	StreamApproxMaxLen *bool
	StreamBlock        time.Duration
	StreamReadCount    int64
	DeadLetterEnabled  *bool
	DeadLetterSuffix   string
	Logger             *logger.Logger
	Options            []pubsub.Option
}

type ReminderComponentConfig struct {
	Enabled         bool
	Notifier        reminder.Notifier
	Name            string
	PollInterval    time.Duration
	BatchSize       int
	ProcessingTTL   time.Duration
	AutoCreateTable *bool
	RetryDelays     []time.Duration
	Logger          *logger.Logger
	Options         []reminder.Option
}

type ComponentsConfig struct {
	PubSub   PubSubComponentConfig
	Reminder ReminderComponentConfig
}

type Profile struct {
	Name string
}

type options struct {
	LogLevel logger.Level

	HttpPort int

	EtcdConfig clientv3.Config

	dsn string

	RedisConfig RedisConfig

	Servers []transport.Server

	components ComponentsConfig

	serviceScheme micro.ServiceScheme

	profile Profile
}

func WithProfile(p Profile) Options {
	return func(o *options) {
		o.profile = p
	}
}

func WithServiceScheme(scheme micro.ServiceScheme) Options {
	return func(o *options) {
		o.serviceScheme = scheme
	}
}

func WithDSN(dsn string) Options {
	return func(o *options) {
		o.dsn = dsn
	}
}

func WithHttpPort(port int) Options {
	return func(o *options) {
		o.HttpPort = port
	}
}

func WithLogLevel(level logger.Level) Options {
	return func(o *options) {
		o.LogLevel = level
	}
}

func WithServers(servers ...transport.Server) Options {
	return func(o *options) {
		o.Servers = servers
	}
}

func WithEtcdConfig(config clientv3.Config) Options {
	return func(o *options) {
		o.EtcdConfig = config
	}
}

func WithRedisConfig(cfg RedisConfig) Options {
	return func(o *options) {
		o.RedisConfig = cfg
	}
}

func WithComponents(cfg ComponentsConfig) Options {
	return func(o *options) {
		o.components.PubSub = mergePubSubComponentConfig(o.components.PubSub, cfg.PubSub)
		o.components.Reminder = mergeReminderComponentConfig(o.components.Reminder, cfg.Reminder)
	}
}

func WithPubSub(opts ...pubsub.Option) Options {
	return func(o *options) {
		WithComponents(ComponentsConfig{
			PubSub: PubSubComponentConfig{
				Enabled: true,
				Options: append([]pubsub.Option(nil), opts...),
			},
		})(o)
	}
}

func WithReminder(notifier reminder.Notifier, opts ...reminder.Option) Options {
	return func(o *options) {
		WithComponents(ComponentsConfig{
			Reminder: ReminderComponentConfig{
				Enabled:  true,
				Notifier: notifier,
				Options:  append([]reminder.Option(nil), opts...),
			},
		})(o)
	}
}

func mergePubSubComponentConfig(current PubSubComponentConfig, incoming PubSubComponentConfig) PubSubComponentConfig {
	merged := current
	if incoming.Enabled {
		merged.Enabled = true
	}
	if incoming.Name != "" {
		merged.Name = incoming.Name
	}
	if incoming.RetryPolicy != nil {
		policy := *incoming.RetryPolicy
		merged.RetryPolicy = &policy
	}
	if incoming.StreamPrefix != "" {
		merged.StreamPrefix = incoming.StreamPrefix
	}
	if incoming.StreamMaxLen > 0 {
		merged.StreamMaxLen = incoming.StreamMaxLen
	}
	if incoming.StreamApproxMaxLen != nil {
		value := *incoming.StreamApproxMaxLen
		merged.StreamApproxMaxLen = &value
	}
	if incoming.StreamBlock > 0 {
		merged.StreamBlock = incoming.StreamBlock
	}
	if incoming.StreamReadCount > 0 {
		merged.StreamReadCount = incoming.StreamReadCount
	}
	if incoming.DeadLetterEnabled != nil {
		value := *incoming.DeadLetterEnabled
		merged.DeadLetterEnabled = &value
	}
	if incoming.DeadLetterSuffix != "" {
		merged.DeadLetterSuffix = incoming.DeadLetterSuffix
	}
	if incoming.Logger != nil {
		merged.Logger = incoming.Logger
	}
	if len(incoming.Options) > 0 {
		merged.Options = append([]pubsub.Option(nil), incoming.Options...)
	}
	return merged
}

func mergeReminderComponentConfig(current ReminderComponentConfig, incoming ReminderComponentConfig) ReminderComponentConfig {
	merged := current
	if incoming.Enabled {
		merged.Enabled = true
	}
	if incoming.Notifier != nil {
		merged.Notifier = incoming.Notifier
	}
	if incoming.Name != "" {
		merged.Name = incoming.Name
	}
	if incoming.PollInterval > 0 {
		merged.PollInterval = incoming.PollInterval
	}
	if incoming.BatchSize > 0 {
		merged.BatchSize = incoming.BatchSize
	}
	if incoming.ProcessingTTL > 0 {
		merged.ProcessingTTL = incoming.ProcessingTTL
	}
	if incoming.AutoCreateTable != nil {
		value := *incoming.AutoCreateTable
		merged.AutoCreateTable = &value
	}
	if len(incoming.RetryDelays) > 0 {
		merged.RetryDelays = append([]time.Duration(nil), incoming.RetryDelays...)
	}
	if incoming.Logger != nil {
		merged.Logger = incoming.Logger
	}
	if len(incoming.Options) > 0 {
		merged.Options = append([]reminder.Option(nil), incoming.Options...)
	}
	return merged
}

func (cfg PubSubComponentConfig) buildOptions() []pubsub.Option {
	options := append([]pubsub.Option(nil), cfg.Options...)
	if cfg.Name != "" {
		options = append(options, pubsub.WithName(cfg.Name))
	}
	if cfg.Logger != nil {
		options = append(options, pubsub.WithLogger(cfg.Logger))
	}
	if cfg.RetryPolicy != nil {
		options = append(options, pubsub.WithRetryPolicy(*cfg.RetryPolicy))
	}
	streamPolicy := pubsub.StreamPolicy{}
	hasStreamPolicy := false
	if cfg.StreamPrefix != "" {
		streamPolicy.Prefix = cfg.StreamPrefix
		hasStreamPolicy = true
	}
	if cfg.StreamMaxLen > 0 {
		streamPolicy.MaxLen = cfg.StreamMaxLen
		hasStreamPolicy = true
	}
	if cfg.StreamApproxMaxLen != nil {
		streamPolicy.ApproxMaxLen = *cfg.StreamApproxMaxLen
		hasStreamPolicy = true
	}
	if cfg.StreamBlock > 0 {
		streamPolicy.Block = cfg.StreamBlock
		hasStreamPolicy = true
	}
	if cfg.StreamReadCount > 0 {
		streamPolicy.ReadCount = cfg.StreamReadCount
		hasStreamPolicy = true
	}
	if hasStreamPolicy {
		options = append(options, pubsub.WithStreamPolicy(streamPolicy))
	}
	deadLetterPolicy := pubsub.DeadLetterPolicy{}
	hasDeadLetterPolicy := false
	if cfg.DeadLetterEnabled != nil {
		deadLetterPolicy.Enabled = *cfg.DeadLetterEnabled
		hasDeadLetterPolicy = true
	}
	if cfg.DeadLetterSuffix != "" {
		deadLetterPolicy.Suffix = cfg.DeadLetterSuffix
		hasDeadLetterPolicy = true
	}
	if hasDeadLetterPolicy {
		options = append(options, pubsub.WithDeadLetterPolicy(deadLetterPolicy))
	}
	return options
}

func (cfg ReminderComponentConfig) buildOptions() []reminder.Option {
	options := append([]reminder.Option(nil), cfg.Options...)
	if cfg.Name != "" {
		options = append(options, reminder.WithName(cfg.Name))
	}
	if cfg.PollInterval > 0 {
		options = append(options, reminder.WithPollInterval(cfg.PollInterval))
	}
	if cfg.BatchSize > 0 {
		options = append(options, reminder.WithBatchSize(cfg.BatchSize))
	}
	if cfg.ProcessingTTL > 0 {
		options = append(options, reminder.WithProcessingTTL(cfg.ProcessingTTL))
	}
	if cfg.AutoCreateTable != nil {
		options = append(options, reminder.WithAutoCreateTable(*cfg.AutoCreateTable))
	}
	if len(cfg.RetryDelays) > 0 {
		options = append(options, reminder.WithRetryDelays(cfg.RetryDelays...))
	}
	if cfg.Logger != nil {
		options = append(options, reminder.WithLogger(cfg.Logger))
	}
	return options
}

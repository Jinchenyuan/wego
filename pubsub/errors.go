package pubsub

import "errors"

var ErrRedisNotConfigured = errors.New("pubsub redis is not configured")
var ErrServiceStarted = errors.New("pubsub service already started")
var ErrInvalidTopic = errors.New("invalid pubsub topic")
var ErrInvalidMessage = errors.New("invalid pubsub message")
var ErrInvalidSubscription = errors.New("invalid pubsub subscription")
var ErrDuplicateSubscription = errors.New("duplicate pubsub subscription")
var ErrHandlerRequired = errors.New("pubsub handler is required")

package pubsub

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

const (
	fieldKey         = "key"
	fieldType        = "type"
	fieldData        = "data"
	fieldHeaders     = "headers"
	fieldPublishedAt = "published_at"
	fieldMaxDeliver  = "max_deliver"
	fieldAttempt     = "attempt"
)

type Message struct {
	Key         string
	Type        string
	Data        []byte
	Headers     map[string]string
	MaxDeliver  int
	ScheduledAt time.Time
}

func (m Message) Validate() error {
	if strings.TrimSpace(m.Type) == "" && len(m.Data) == 0 {
		return ErrInvalidMessage
	}
	if m.MaxDeliver < 0 {
		return ErrInvalidMessage
	}
	if !m.ScheduledAt.IsZero() {
		return ErrInvalidMessage
	}
	return nil
}

type Delivery struct {
	ID          string
	Topic       string
	Group       string
	Consumer    string
	Attempt     int
	PublishedAt time.Time
	Message     Message
	Raw         map[string]string
}

type Handler interface {
	Handle(context.Context, *Delivery) error
}

type HandlerFunc func(context.Context, *Delivery) error

func (f HandlerFunc) Handle(ctx context.Context, delivery *Delivery) error {
	return f(ctx, delivery)
}

type Entry struct {
	ID     string
	Values map[string]string
}

func messageFields(msg Message, publishedAt time.Time, defaultMaxDeliver int) (map[string]any, error) {
	return messageFieldsWithAttempt(msg, publishedAt, defaultMaxDeliver, 1)
}

func messageFieldsWithAttempt(msg Message, publishedAt time.Time, defaultMaxDeliver int, attempt int) (map[string]any, error) {
	if err := msg.Validate(); err != nil {
		return nil, err
	}
	if attempt <= 0 {
		return nil, ErrInvalidMessage
	}

	maxDeliver := msg.MaxDeliver
	if maxDeliver <= 0 {
		maxDeliver = defaultMaxDeliver
	}

	headers, err := json.Marshal(msg.Headers)
	if err != nil {
		return nil, ErrInvalidMessage
	}

	return map[string]any{
		fieldKey:         msg.Key,
		fieldType:        msg.Type,
		fieldData:        string(msg.Data),
		fieldHeaders:     string(headers),
		fieldPublishedAt: publishedAt.UTC().Format(time.RFC3339Nano),
		fieldMaxDeliver:  strconv.Itoa(maxDeliver),
		fieldAttempt:     strconv.Itoa(attempt),
	}, nil
}

func decodeEntry(topic string, group string, consumer string, entry Entry) (*Delivery, error) {
	if entry.ID == "" {
		return nil, ErrInvalidMessage
	}

	maxDeliver, err := strconv.Atoi(strings.TrimSpace(entry.Values[fieldMaxDeliver]))
	if err != nil || maxDeliver < 0 {
		return nil, ErrInvalidMessage
	}

	attempt := 1
	if rawAttempt := strings.TrimSpace(entry.Values[fieldAttempt]); rawAttempt != "" {
		attempt, err = strconv.Atoi(rawAttempt)
		if err != nil || attempt <= 0 {
			return nil, ErrInvalidMessage
		}
	}

	publishedAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(entry.Values[fieldPublishedAt]))
	if err != nil {
		return nil, ErrInvalidMessage
	}

	headers := map[string]string{}
	rawHeaders := strings.TrimSpace(entry.Values[fieldHeaders])
	if rawHeaders != "" {
		if err := json.Unmarshal([]byte(rawHeaders), &headers); err != nil {
			return nil, ErrInvalidMessage
		}
	}

	message := Message{
		Key:        entry.Values[fieldKey],
		Type:       entry.Values[fieldType],
		Data:       []byte(entry.Values[fieldData]),
		Headers:    headers,
		MaxDeliver: maxDeliver,
	}

	return &Delivery{
		ID:          entry.ID,
		Topic:       topic,
		Group:       group,
		Consumer:    consumer,
		Attempt:     attempt,
		PublishedAt: publishedAt,
		Message:     message,
		Raw:         entry.Values,
	}, nil
}

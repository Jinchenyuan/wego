package reminder

import "time"

type Status string

const (
	StatusPending    Status = "pending"
	StatusProcessing Status = "processing"
	StatusSent       Status = "sent"
	StatusFailed     Status = "failed"
	StatusCanceled   Status = "canceled"
)

type Reminder struct {
	tableName  struct{}  `bun:"table:reminders,alias:r"`
	ID         int64     `bun:",pk,autoincrement"`
	Key        string    `bun:",notnull,unique"`
	UserID     string    `bun:",notnull"`
	Channel    string    `bun:",notnull"`
	Payload    string    `bun:",notnull"`
	ScheduleAt time.Time `bun:",notnull"`
	Status     Status    `bun:",notnull"`
	RetryCount int       `bun:",notnull,default:0"`
	MaxRetry   int       `bun:",notnull,default:3"`
	LastError  string
	SentAt     *time.Time `bun:",nullzero"`
	CanceledAt *time.Time `bun:",nullzero"`
	CreatedAt  time.Time  `bun:",notnull"`
	UpdatedAt  time.Time  `bun:",notnull"`
}

type CreateParams struct {
	Key        string
	UserID     string
	Channel    string
	Payload    string
	ScheduleAt time.Time
	MaxRetry   int
}

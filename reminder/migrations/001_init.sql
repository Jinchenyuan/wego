CREATE TABLE IF NOT EXISTS reminders (
    id BIGSERIAL PRIMARY KEY,
    key TEXT NOT NULL UNIQUE,
    user_id TEXT NOT NULL,
    channel TEXT NOT NULL,
    payload TEXT NOT NULL,
    schedule_at TIMESTAMPTZ NOT NULL,
    status TEXT NOT NULL,
    retry_count INTEGER NOT NULL DEFAULT 0,
    max_retry INTEGER NOT NULL DEFAULT 3,
    last_error TEXT NOT NULL DEFAULT '',
    sent_at TIMESTAMPTZ NULL,
    canceled_at TIMESTAMPTZ NULL,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX IF NOT EXISTS reminders_worker_idx ON reminders (status, schedule_at, id);
CREATE INDEX IF NOT EXISTS reminders_user_idx ON reminders (user_id, schedule_at, id);
CREATE INDEX IF NOT EXISTS reminders_channel_status_idx ON reminders (channel, status, schedule_at, id);

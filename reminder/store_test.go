package reminder

import (
	"testing"
	"time"
)

func TestCursorRoundTrip(t *testing.T) {
	want := listCursor{ScheduleAt: time.Date(2026, 8, 12, 8, 0, 0, 123, time.UTC), ID: 42}
	got, err := decodeCursor(encodeCursor(want))
	if err != nil {
		t.Fatalf("decodeCursor: %v", err)
	}
	if !got.ScheduleAt.Equal(want.ScheduleAt) || got.ID != want.ID {
		t.Fatalf("cursor = %#v, want %#v", got, want)
	}
}

func TestCursorRejectsInvalidValues(t *testing.T) {
	for _, raw := range []string{"invalid", encodeCursor(listCursor{})} {
		if _, err := decodeCursor(raw); err == nil {
			t.Fatalf("expected invalid cursor for %q", raw)
		}
	}
}

func TestValidStatus(t *testing.T) {
	if !validStatus(StatusPending) {
		t.Fatal("pending should be valid")
	}
	if validStatus(Status("unknown")) {
		t.Fatal("unknown status should be invalid")
	}
}

package reminder

import (
	"testing"
	"time"
)

func TestRetryDelayFallsBackToLastBucket(t *testing.T) {
	svc := &Service{
		opts: options{
			retryDelays: []time.Duration{time.Second, 2 * time.Second, 3 * time.Second},
		},
	}

	if got := svc.retryDelay(1); got != time.Second {
		t.Fatalf("expected first retry delay, got %s", got)
	}

	if got := svc.retryDelay(5); got != 3*time.Second {
		t.Fatalf("expected last retry delay bucket, got %s", got)
	}
}

func TestDefaultOptions(t *testing.T) {
	opts := defaultOptions()

	if opts.name != "reminder" {
		t.Fatalf("expected default name reminder, got %q", opts.name)
	}
	if opts.pollInterval <= 0 {
		t.Fatalf("expected positive poll interval, got %s", opts.pollInterval)
	}
	if opts.batchSize <= 0 {
		t.Fatalf("expected positive batch size, got %d", opts.batchSize)
	}
	if opts.processingTTL <= 0 {
		t.Fatalf("expected positive processing ttl, got %s", opts.processingTTL)
	}
}

func TestWithProcessingTTL(t *testing.T) {
	opts := defaultOptions()
	WithProcessingTTL(15 * time.Second)(&opts)

	if opts.processingTTL != 15*time.Second {
		t.Fatalf("expected processing ttl to be updated, got %s", opts.processingTTL)
	}
}

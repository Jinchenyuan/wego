package logger

import (
	"bytes"
	"encoding/json"
	"log"
	"testing"
)

func TestGetLoggerReturnsSharedInstanceForService(t *testing.T) {
	serviceName := "test-service-shared"
	loggerRegistry.Delete(serviceName)
	t.Cleanup(func() {
		loggerRegistry.Delete(serviceName)
	})

	first := GetLogger(serviceName)
	second := GetLogger(serviceName)

	if first != second {
		t.Fatalf("expected shared logger instance for service %q", serviceName)
	}
}

func TestGetLoggerUsesDefaultServiceName(t *testing.T) {
	serviceName := "default"
	loggerRegistry.Delete(serviceName)
	t.Cleanup(func() {
		loggerRegistry.Delete(serviceName)
	})

	l := GetLogger("  ")
	if l.serviceName != serviceName {
		t.Fatalf("expected default service name %q, got %q", serviceName, l.serviceName)
	}
}

func TestLoggerSetLevelAndMethodsFilter(t *testing.T) {
	l := NewLogger("account-api")

	var buf bytes.Buffer
	l.base = log.New(&buf, "", 0)
	l.SetLevel(Warn)

	l.Info("ignore this log")
	l.Error("emit this log")

	output := buf.String()
	if bytes.Contains([]byte(output), []byte("ignore this log")) {
		t.Fatalf("expected info log to be filtered out, output: %q", output)
	}
	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &record); err != nil {
		t.Fatalf("expected JSON log: %v", err)
	}
	if record["service"] != "account-api" || record["level"] != "ERROR" || record["message"] != "emit this log" || record["timestamp"] == "" {
		t.Fatalf("unexpected log record: %#v", record)
	}
}

func TestLoggerMethodsEmitJSON(t *testing.T) {
	l := NewLogger("account-api")

	var buf bytes.Buffer
	l.base = log.New(&buf, "", 0)
	l.SetLevel(Debug)

	l.Warn("warn log")
	l.Error("error log")

	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	if len(lines) != 2 {
		t.Fatalf("expected two log records, got %q", buf.String())
	}
	for _, line := range lines {
		var record map[string]any
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatalf("expected JSON log: %v", err)
		}
	}
}

func TestLoggerFatalCallsExit(t *testing.T) {
	l := NewLogger("account-api")

	var buf bytes.Buffer
	l.base = log.New(&buf, "", 0)
	l.SetLevel(Debug)

	originalExit := exitProcess
	exitCode := -1
	exitProcess = func(code int) {
		exitCode = code
	}
	t.Cleanup(func() {
		exitProcess = originalExit
	})

	l.Fatal("fatal log")

	if exitCode != 1 {
		t.Fatalf("expected exit code 1, got %d", exitCode)
	}

	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &record); err != nil {
		t.Fatalf("expected JSON log: %v", err)
	}
	if record["level"] != "FATAL" || record["message"] != "fatal log" {
		t.Fatalf("unexpected fatal record: %#v", record)
	}
}

package logger

import (
	"bytes"
	"log"
	"regexp"
	"strings"
	"testing"
)

var ansiRegexp = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func stripANSI(s string) string {
	return ansiRegexp.ReplaceAllString(s, "")
}

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
	plainOutput := stripANSI(output)
	if strings.Contains(output, "ignore this log") {
		t.Fatalf("expected info log to be filtered out, output: %q", output)
	}

	if !strings.Contains(plainOutput, "[account-api ERROR]:") {
		t.Fatalf("expected output with service/level prefix [account-api ERROR]:, got %q", output)
	}

	matched, err := regexp.MatchString(`^\[\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\] \[account-api ERROR\]:`, plainOutput)
	if err != nil {
		t.Fatalf("failed to match timestamp prefix: %v", err)
	}
	if !matched {
		t.Fatalf("expected timestamp at the beginning of prefix, got %q", output)
	}

	if !strings.Contains(output, "emit this log") {
		t.Fatalf("expected error message to be printed, output: %q", output)
	}
}

func TestLoggerMethodsColorsByLevel(t *testing.T) {
	l := NewLogger("account-api")

	var buf bytes.Buffer
	l.base = log.New(&buf, "", 0)
	l.SetLevel(Debug)

	l.Warn("warn log")
	l.Error("error log")

	output := buf.String()
	if !strings.Contains(output, "\x1b[33m") {
		t.Fatalf("expected WARN log to include yellow color code, output: %q", output)
	}
	if !strings.Contains(output, "\x1b[31m") {
		t.Fatalf("expected ERROR log to include red color code, output: %q", output)
	}
	if !strings.Contains(output, "\x1b[0m") {
		t.Fatalf("expected colored output to include ANSI reset code, output: %q", output)
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

	plainOutput := stripANSI(buf.String())
	if !strings.Contains(plainOutput, "[account-api FATAL]:") {
		t.Fatalf("expected fatal prefix in output, got %q", plainOutput)
	}
	if !strings.Contains(plainOutput, "fatal log") {
		t.Fatalf("expected fatal message in output, got %q", plainOutput)
	}
}

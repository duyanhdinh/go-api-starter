package logger

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestFormatAndLevel(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		environment string
		level       slog.Level
		wantDebug   bool
		wantInfo    bool
	}{
		{"dev", "dev", slog.LevelDebug, true, true},
		{"test", "test", slog.LevelDebug, true, true},
		{"prod", "prod", slog.LevelInfo, false, true},
		{"prod_debug", "prod", slog.LevelDebug, true, true},
		{"dev_warn", "dev", slog.LevelWarn, false, false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var output bytes.Buffer
			applicationLogger := newLogger(&output, testCase.environment, testCase.level)
			applicationLogger.Debug("debug message")
			applicationLogger.Info("info message")
			applicationLogger.Warn("warn message")
			applicationLogger.Error("error message")
			for message, expected := range map[string]bool{
				"debug message": testCase.wantDebug, "info message": testCase.wantInfo,
				"warn message": true, "error message": true,
			} {
				if strings.Contains(output.String(), message) != expected {
					t.Fatalf("unexpected level filtering: %s", output.String())
				}
			}
			for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
				if testCase.environment == "prod" {
					var record map[string]any
					if err := json.Unmarshal([]byte(line), &record); err != nil {
						t.Fatal(err)
					}
					if record["time"] == nil || record["level"] == nil || record["msg"] == nil {
						t.Fatalf("missing standard fields: %s", line)
					}
				} else if !strings.HasPrefix(line, "time=") || !strings.Contains(line, " level=") {
					t.Fatalf("expected text format: %s", line)
				}
			}
		})
	}
}

func TestStructuredFieldsAndChildIsolation(t *testing.T) {
	var output bytes.Buffer
	parent := newLogger(&output, "prod", slog.LevelInfo)
	child := parent.With("module", "order")
	child.Info("created", "order_id", 42, slog.Group("result", "status", "ok"))
	parent.Info("parent")
	decoder := json.NewDecoder(&output)
	var record map[string]any
	if err := decoder.Decode(&record); err != nil {
		t.Fatal(err)
	}
	if record["module"] != "order" || record["order_id"] != float64(42) {
		t.Fatalf("structured fields not preserved: %v", record)
	}
	group, ok := record["result"].(map[string]any)
	if !ok || group["status"] != "ok" {
		t.Fatalf("group not preserved: %v", record)
	}
	record = nil
	if err := decoder.Decode(&record); err != nil {
		t.Fatal(err)
	}
	if _, exists := record["module"]; exists {
		t.Fatal("child fields leaked into parent")
	}
}

func TestNewWritesToStdout(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	originalStdout := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = originalStdout }()
	New("prod", slog.LevelInfo).Info("stdout message")
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(output, []byte("stdout message")) || !json.Valid(bytes.TrimSpace(output)) {
		t.Fatalf("expected JSON on stdout: %s", output)
	}
}

package log

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestNew_JSONLevel(t *testing.T) {
	var buf bytes.Buffer
	l, err := New("warn", "json", &buf)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	l.Info("dropped")
	l.Warn("kept", "k", "v")

	out := strings.TrimSpace(buf.String())
	if strings.Contains(out, "dropped") {
		t.Errorf("info-level line should be filtered at warn: %q", out)
	}
	if !strings.Contains(out, "kept") {
		t.Errorf("warn-level line missing: %q", out)
	}

	var rec map[string]any
	if err := json.Unmarshal([]byte(out), &rec); err != nil {
		t.Fatalf("output is not JSON: %v: %q", err, out)
	}
	if rec["msg"] != "kept" || rec["k"] != "v" {
		t.Errorf("unexpected record: %v", rec)
	}
}

func TestNew_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	l, err := New("info", "text", &buf)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	l.Info("hello")
	if !strings.Contains(buf.String(), "msg=hello") {
		t.Errorf("text format missing msg=hello: %q", buf.String())
	}
}

func TestNew_DefaultsToJSONInfo(t *testing.T) {
	var buf bytes.Buffer
	l, err := New("", "", &buf)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	l.Debug("filtered")
	l.Info("kept")
	if strings.Contains(buf.String(), "filtered") {
		t.Errorf("debug should be filtered at default info level: %q", buf.String())
	}
	if !strings.Contains(buf.String(), `"msg":"kept"`) {
		t.Errorf("default format should be JSON: %q", buf.String())
	}
}

func TestNew_BadLevel(t *testing.T) {
	if _, err := New("loud", "json", nil); err == nil {
		t.Fatal("expected error for invalid level")
	}
}

func TestNew_BadFormat(t *testing.T) {
	if _, err := New("info", "yaml", nil); err == nil {
		t.Fatal("expected error for invalid format")
	}
}

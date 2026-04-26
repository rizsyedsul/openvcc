package chaos

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriterSink_Encodes(t *testing.T) {
	var buf bytes.Buffer
	s := NewWriterSink(&buf)
	r := &Report{Result: "pass", FailedCloud: "aws"}
	if err := s.Write(r); err != nil {
		t.Fatalf("Write: %v", err)
	}
	var got Report
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Result != "pass" || got.FailedCloud != "aws" {
		t.Errorf("round-trip lost data: %+v", got)
	}
}

func TestFileSink_WritesTimestampedFile(t *testing.T) {
	dir := t.TempDir()
	s := &FileSink{Dir: filepath.Join(dir, "reports"), Prefix: "test"}
	r := &Report{
		EndedAt:     time.Date(2026, 4, 26, 3, 0, 0, 0, time.UTC),
		FailedCloud: "aws",
		Result:      "pass",
	}
	if err := s.Write(r); err != nil {
		t.Fatalf("Write: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(dir, "reports"))
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 file, got %d", len(entries))
	}
	name := entries[0].Name()
	if !strings.HasPrefix(name, "test-") || !strings.HasSuffix(name, ".json") {
		t.Errorf("filename=%q", name)
	}
	if !strings.Contains(name, "2026-04-26T03-00-00Z") {
		t.Errorf("filename missing timestamp: %q", name)
	}
}

func TestFileSink_DefaultPrefix(t *testing.T) {
	dir := t.TempDir()
	s := &FileSink{Dir: dir}
	if err := s.Write(&Report{EndedAt: time.Now()}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || !strings.HasPrefix(entries[0].Name(), "openvcc-chaos-") {
		t.Errorf("default prefix not applied: %v", entries)
	}
}

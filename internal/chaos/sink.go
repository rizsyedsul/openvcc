package chaos

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Sink receives a finished Report. Implementations decide how to persist it
// (local file, S3-compatible blob store, HTTP POST, etc.).
type Sink interface {
	Write(*Report) error
}

// SinkFunc adapts a function to the Sink interface.
type SinkFunc func(*Report) error

func (f SinkFunc) Write(r *Report) error { return f(r) }

// WriterSink writes the report as indented JSON to w. Goroutine-safe.
type WriterSink struct {
	mu sync.Mutex
	w  io.Writer
}

func NewWriterSink(w io.Writer) *WriterSink { return &WriterSink{w: w} }

func (s *WriterSink) Write(r *Report) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	enc := json.NewEncoder(s.w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// FileSink writes one JSON file per Report under Dir, named
// <Prefix>-<RFC3339-timestamp>.json. Useful for the schedule subcommand or
// any cron-driven pipeline.
type FileSink struct {
	Dir    string
	Prefix string
}

func (s *FileSink) Write(r *Report) error {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", s.Dir, err)
	}
	prefix := s.Prefix
	if prefix == "" {
		prefix = "openvcc-chaos"
	}
	stamp := r.EndedAt.UTC().Format("2006-01-02T15-04-05Z")
	if r.EndedAt.IsZero() {
		stamp = time.Now().UTC().Format("2006-01-02T15-04-05Z")
	}
	path := filepath.Join(s.Dir, fmt.Sprintf("%s-%s.json", prefix, stamp))
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

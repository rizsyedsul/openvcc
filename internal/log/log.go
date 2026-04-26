package log

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

type Format string

const (
	FormatJSON Format = "json"
	FormatText Format = "text"
)

func New(level, format string, w io.Writer) (*slog.Logger, error) {
	if w == nil {
		w = os.Stderr
	}
	lvl, err := parseLevel(level)
	if err != nil {
		return nil, err
	}
	opts := &slog.HandlerOptions{Level: lvl}

	var h slog.Handler
	switch Format(strings.ToLower(format)) {
	case FormatJSON, "":
		h = slog.NewJSONHandler(w, opts)
	case FormatText:
		h = slog.NewTextHandler(w, opts)
	default:
		return nil, fmt.Errorf("unknown log format %q (want json|text)", format)
	}
	return slog.New(h), nil
}

func parseLevel(s string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "info":
		return slog.LevelInfo, nil
	case "debug":
		return slog.LevelDebug, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unknown log level %q (want debug|info|warn|error)", s)
	}
}

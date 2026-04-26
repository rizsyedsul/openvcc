// Package engine is the public Go API for embedding Open VCC in your own
// program. The exposed surface is intentionally small — most users should
// run the openvcc binary instead. Use this package when you need to embed
// the engine inside a larger Go process.
package engine

import (
	"context"
	"log/slog"

	"github.com/syedsumx/openvcc/internal/config"
	"github.com/syedsumx/openvcc/internal/engine"
)

type Config = config.Config

func LoadConfig(path string) (*Config, error) { return config.Load(path) }

func ParseConfig(data []byte) (*Config, error) { return config.Parse(data) }

type Engine struct{ inner *engine.Engine }

func New(cfg *Config, configPath string, log *slog.Logger) (*Engine, error) {
	e, err := engine.New(cfg, configPath, log)
	if err != nil {
		return nil, err
	}
	return &Engine{inner: e}, nil
}

func (e *Engine) Run(ctx context.Context) error    { return e.inner.Run(ctx) }
func (e *Engine) Reload(ctx context.Context) error { return e.inner.Reload(ctx) }

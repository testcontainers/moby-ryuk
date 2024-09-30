package main

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/caarlos0/env/v11"
)

// config represents the configuration for the reaper.
type config struct {
	// ConnectionTimeout is the duration without receiving any connections which will trigger a shutdown.
	ConnectionTimeout time.Duration `env:"RYUK_CONNECTION_TIMEOUT" envDefault:"60s"`

	// ReconnectionTimeout is the duration after the last connection closes which will trigger
	// resource clean up and shutdown.
	ReconnectionTimeout time.Duration `env:"RYUK_RECONNECTION_TIMEOUT" envDefault:"10s"`

	// ShutdownTimeout is the maximum amount of time the reaper will wait
	// for once signalled to shutdown before it terminates even if connections
	// are still established.
	ShutdownTimeout time.Duration `env:"RYUK_SHUTDOWN_TIMEOUT" envDefault:"10m"`

	// Port is the port to listen on for connections.
	Port uint16 `env:"RYUK_PORT" envDefault:"8080"`

	// Verbose is whether to enable verbose aka debug logging.
	Verbose bool `env:"RYUK_VERBOSE" envDefault:"false"`
}

// LogAttrs returns the configuration as a slice of attributes.
func (c config) LogAttrs() []slog.Attr {
	return []slog.Attr{
		slog.Duration("connection_timeout", c.ConnectionTimeout),
		slog.Duration("reconnection_timeout", c.ReconnectionTimeout),
		slog.Duration("shutdown_timeout", c.ShutdownTimeout),
		slog.Int("port", int(c.Port)),
		slog.Bool("verbose", c.Verbose),
	}
}

// loadConfig loads the configuration from the environment
// applying defaults where necessary.
func loadConfig() (*config, error) {
	var cfg config
	if err := env.Parse(&cfg); err != nil {
		return nil, fmt.Errorf("parse env: %w", err)
	}

	return &cfg, nil
}

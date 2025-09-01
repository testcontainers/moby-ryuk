package main

import (
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// clearConfigEnv clears the environment variables for the config fields.
func clearConfigEnv(t *testing.T) {
	t.Helper()

	var cfg config

	typ := reflect.TypeOf(cfg)
	for i := range typ.NumField() {
		field := typ.Field(i)
		if name := field.Tag.Get("env"); name != "" {
			if os.Getenv(name) != "" {
				t.Setenv(name, "")
			}
		}
	}
}

func Test_loadConfig(t *testing.T) {
	clearConfigEnv(t)

	t.Run("defaults", func(t *testing.T) {
		expected := config{
			Port:                 8080,
			ConnectionTimeout:    time.Minute,
			ReconnectionTimeout:  time.Second * 10,
			ShutdownTimeout:      time.Minute * 10,
			RemoveRetries:        10,
			RequestTimeout:       time.Second * 10,
			RetryOffset:          -time.Second,
			ChangesRetryInterval: time.Second,
		}

		cfg, err := loadConfig()
		require.NoError(t, err)
		require.Equal(t, expected, *cfg)
	})

	t.Run("custom", func(t *testing.T) {
		t.Setenv("RYUK_PORT", "1234")
		t.Setenv("RYUK_CONNECTION_TIMEOUT", "2s")
		t.Setenv("RYUK_RECONNECTION_TIMEOUT", "3s")
		t.Setenv("RYUK_SHUTDOWN_TIMEOUT", "7s")
		t.Setenv("RYUK_VERBOSE", "true")
		t.Setenv("RYUK_REQUEST_TIMEOUT", "4s")
		t.Setenv("RYUK_REMOVE_RETRIES", "5")
		t.Setenv("RYUK_RETRY_OFFSET", "-6s")
		t.Setenv("RYUK_CHANGES_RETRY_INTERVAL", "8s")

		expected := config{
			Port:                 1234,
			ConnectionTimeout:    time.Second * 2,
			ReconnectionTimeout:  time.Second * 3,
			ShutdownTimeout:      time.Second * 7,
			Verbose:              true,
			RemoveRetries:        5,
			RequestTimeout:       time.Second * 4,
			RetryOffset:          -time.Second * 6,
			ChangesRetryInterval: time.Second * 8,
		}

		cfg, err := loadConfig()
		require.NoError(t, err)
		require.Equal(t, expected, *cfg)
	})

	for _, name := range []string{
		"RYUK_PORT",
		"RYUK_CONNECTION_TIMEOUT",
		"RYUK_RECONNECTION_TIMEOUT",
		"RYUK_SHUTDOWN_TIMEOUT",
		"RYUK_VERBOSE",
		"RYUK_REQUEST_TIMEOUT",
		"RYUK_REMOVE_RETRIES",
		"RYUK_RETRY_OFFSET",
	} {
		t.Run("invalid-"+name, func(t *testing.T) {
			t.Setenv(name, "invalid")

			_, err := loadConfig()
			require.Error(t, err)
		})
	}
}

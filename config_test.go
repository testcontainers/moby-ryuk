package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func Test_loadConfig(t *testing.T) {
	tests := map[string]struct {
		setEnv   func(*testing.T)
		expected config
	}{
		"defaults": {
			expected: config{
				ConnectionTimeout:   time.Minute,
				Port:                8080,
				ReconnectionTimeout: time.Second * 10,
				RemoveRetries:       10,
				RequestTimeout:      time.Second * 10,
				RetryOffset:         -time.Second,
				ShutdownTimeout:     time.Minute * 10,
			},
		},
		"custom": {
			setEnv: func(t *testing.T) {
				t.Helper()
				t.Setenv("RYUK_PORT", "1234")
				t.Setenv("RYUK_CONNECTION_TIMEOUT", "2s")
				t.Setenv("RYUK_RECONNECTION_TIMEOUT", "3s")
				t.Setenv("RYUK_REQUEST_TIMEOUT", "4s")
				t.Setenv("RYUK_REMOVE_RETRIES", "5")
				t.Setenv("RYUK_RETRY_OFFSET", "-6s")
				t.Setenv("RYUK_SHUTDOWN_TIMEOUT", "7s")
			},
			expected: config{
				Port:                1234,
				ConnectionTimeout:   time.Second * 2,
				ReconnectionTimeout: time.Second * 3,
				RequestTimeout:      time.Second * 4,
				RemoveRetries:       5,
				RetryOffset:         -time.Second * 6,
				ShutdownTimeout:     time.Second * 7,
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if tc.setEnv != nil {
				tc.setEnv(t)
			}

			cfg, err := loadConfig()
			require.NoError(t, err)
			require.Equal(t, tc.expected, *cfg)
		})
	}
}

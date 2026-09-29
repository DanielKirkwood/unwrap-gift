package logger_test

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/DanielKirkwood/unwrap-gift/internal/clients/logger"
)

func TestNew(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		cfg         logger.Config
		wantErr     bool
		wantEnabled slog.Level // level that Enabled() should report true for
		wantJSON    bool
	}{
		{
			name:        "prod info level uses json handler",
			cfg:         logger.Config{Dev: false, Level: "info"},
			wantEnabled: slog.LevelInfo,
			wantJSON:    true,
		},
		{
			name:        "prod respects warn level, debug not enabled",
			cfg:         logger.Config{Dev: false, Level: "warn"},
			wantEnabled: slog.LevelWarn,
			wantJSON:    true,
		},
		{
			name:        "dev floors warn to debug and uses text handler",
			cfg:         logger.Config{Dev: true, Level: "warn"},
			wantEnabled: slog.LevelDebug,
			wantJSON:    false,
		},
		{
			name:        "dev leaves debug as debug",
			cfg:         logger.Config{Dev: true, Level: "debug"},
			wantEnabled: slog.LevelDebug,
			wantJSON:    false,
		},
		{
			name:    "unknown level errors",
			cfg:     logger.Config{Level: "trace"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var buf bytes.Buffer
			tt.cfg.Writer = &buf

			got, err := logger.New(tt.cfg)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("New() error = nil, want error")
				}
				return
			}
			if err != nil {
				t.Fatalf("New() error = %v, want nil", err)
			}

			if !got.Enabled(t.Context(), tt.wantEnabled) {
				t.Errorf("logger not enabled for level %v", tt.wantEnabled)
			}
			if got.Enabled(t.Context(), tt.wantEnabled-1) {
				t.Errorf("logger unexpectedly enabled for level below %v", tt.wantEnabled)
			}

			got.Log(t.Context(), tt.wantEnabled, "hello")

			isJSON := strings.HasPrefix(strings.TrimSpace(buf.String()), "{")
			if isJSON != tt.wantJSON {
				t.Errorf("output JSON-shaped = %v, want %v (output: %q)", isJSON, tt.wantJSON, buf.String())
			}
		})
	}
}

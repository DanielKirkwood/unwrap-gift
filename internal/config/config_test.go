package config_test

import (
	"testing"

	"github.com/DanielKirkwood/unwrap-gift/internal/config"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want config.EnvVars
	}{
		{
			name: "defaults when unset",
			env:  map[string]string{},
			want: config.EnvVars{
				Env: "development", LogLevel: "info",
				PublicPort: "8080", ProtectedPort: "8081", HiddenPort: "8082",
			},
		},
		{
			name: "env vars override defaults",
			env: map[string]string{
				"ENV":            "production",
				"LOG_LEVEL":      "warn",
				"PUBLIC_PORT":    "9090",
				"PROTECTED_PORT": "9091",
				"HIDDEN_PORT":    "9092",
			},
			want: config.EnvVars{
				Env: "production", LogLevel: "warn",
				PublicPort: "9090", ProtectedPort: "9091", HiddenPort: "9092",
			},
		},
		{
			name: "optional fields have no default",
			env: map[string]string{
				"DATABASE_PATH": "/tmp/app.db",
			},
			want: config.EnvVars{
				Env: "development", LogLevel: "info",
				PublicPort: "8080", ProtectedPort: "8081", HiddenPort: "8082",
				DatabasePath: "/tmp/app.db",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			got, err := config.Load()
			if err != nil {
				t.Fatalf("Load() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("Load() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

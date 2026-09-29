// Package logger constructs the application's structured [slog.Logger]:
// JSON output for production, tint-colorized human-readable output for
// development, with color automatically disabled when the destination
// isn't a terminal.
package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
)

// Config controls logger construction. Writer defaults to [os.Stdout] when
// nil.
type Config struct {
	// Dev, when true, floors the effective level at slog.LevelDebug and
	// switches to a human-readable text handler with source locations.
	Dev bool

	// Level is the minimum log level to emit: "debug", "info", "warn", or
	// "error" (case-insensitive).
	Level string

	// Writer is the destination for log output; nil defaults to
	// [os.Stdout].
	Writer io.Writer
}

// New builds a [slog.Logger] from cfg. It returns an error if cfg.Level does
// not name a known level.
func New(cfg Config) (*slog.Logger, error) {
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
		return nil, fmt.Errorf("logger: unknown level %q: %w", cfg.Level, err)
	}

	w := cfg.Writer
	if w == nil {
		w = os.Stdout
	}

	if cfg.Dev && level > slog.LevelDebug {
		level = slog.LevelDebug
	}

	var handler slog.Handler
	if cfg.Dev {
		handler = tint.NewTextHandler(w, &tint.Options{Level: level, AddSource: true, NoColor: !isTerminal(w)})
	} else {
		handler = slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})
	}

	return slog.New(handler), nil
}

// isTerminal reports whether w is a terminal, so tint only colorizes output
// that's actually going to a terminal — not a redirected file or the
// [bytes.Buffer] tests write to.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	return ok && isatty.IsTerminal(f.Fd())
}

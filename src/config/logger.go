package config

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
)

const (
	colorReset  = "\033[0m"
	colorGray   = "\033[90m"
	colorBlue   = "\033[34m"
	colorYellow = "\033[33m"
	colorRed    = "\033[31m"
	colorBold   = "\033[1m"
)

type PrettyLogger struct {
	mu    sync.Mutex
	out   io.Writer
	level slog.Level
	attrs []slog.Attr
}

func NewPrettyLogger(out io.Writer, level slog.Level) *PrettyLogger {
	return &PrettyLogger{out: out, level: level}
}

func (h *PrettyLogger) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *PrettyLogger) Handle(_ context.Context, r slog.Record) error {
	level := r.Level.String()
	var levelColor string
	switch r.Level {
	case slog.LevelDebug:
		levelColor = colorGray
	case slog.LevelInfo:
		levelColor = colorBlue
	case slog.LevelWarn:
		levelColor = colorYellow
	case slog.LevelError:
		levelColor = colorRed + colorBold
	}

	timestamp := r.Time.Format("15:04:05.000")

	// coleta atributos
	var fields []string
	// atributos fixos do logger (WithAttrs)
	for _, a := range h.attrs {
		fields = append(fields, fmt.Sprintf("%s%s%s=%v", colorGray, a.Key, colorReset, a.Value))
	}
	// atributos do record
	r.Attrs(func(a slog.Attr) bool {
		fields = append(fields, fmt.Sprintf("%s%s%s=%v", colorGray, a.Key, colorReset, a.Value))
		return true
	})

	suffix := ""
	if len(fields) > 0 {
		suffix = "  " + strings.Join(fields, " ")
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	fmt.Fprintf(h.out, "%s%s%s  %s%-5s%s  %s%s\n",
		colorGray, timestamp, colorReset,
		levelColor, level, colorReset,
		r.Message,
		suffix,
	)
	return nil
}

func (h *PrettyLogger) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)
	return &PrettyLogger{out: h.out, level: h.level, attrs: newAttrs}
}

func (h *PrettyLogger) WithGroup(name string) slog.Handler {
	return h
}

package logging

import (
	"log/slog"
	"os"
)

func New(service string) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey && len(groups) == 0 {
				a.Key = "timestamp"
			}
			return a
		},
	})).With(slog.String("service", service))
}

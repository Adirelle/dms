package logging

import (
	"context"
	"log"
	"net/http"
)

type contextKey int

var loggerKey = contextKey(1)

// WithLogger creates a Context with the Logger
func WithLogger(ctx context.Context, l Logger) context.Context {
	return context.WithValue(ctx, loggerKey, l)
}

// FromContext gets the Logger from the Context
func FromContext(ctx context.Context, def Logger) Logger {
	if l, ok := ctx.Value(loggerKey).(Logger); ok {
		return l
	}
	return def
}

// FromContext gets the Logger from the Context
func MustFromContext(ctx context.Context) Logger {
	if l := FromContext(ctx, nil); l != nil {
		return l
	}
	log.Panic("logging.FromContext on a Context without a logger !")
	return nil
}

// AddLogger returns an HTTP middleware that injects the given logger to the request context
func AddLogger(logger Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(WithLogger(r.Context(), logger)))
		})
	}
}

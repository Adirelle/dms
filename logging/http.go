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
func FromContext(ctx context.Context) (logger Logger) {
	logger, _ = ctx.Value(loggerKey).(Logger)
	return
}

// FromContext gets the Logger from the Context
func MustFromContext(ctx context.Context) (logger Logger) {
	logger = FromContext(ctx)
	if logger == nil {
		log.Panic("logging.FromContext on a Context without a logger !")
	}
	return
}

// AddLogger returns an HTTP middleware that injects the given logger to the request context
func AddLogger(logger Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(WithLogger(r.Context(), logger)))
		})
	}
}

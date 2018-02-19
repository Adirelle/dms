package logging

import (
	"context"
	"math/rand"
	"net/http"
	"strconv"
)

type contextKey int

var loggerKey = contextKey(1)

func ContextWithLogger(ctx context.Context, l Logger) context.Context {
	return context.WithValue(ctx, loggerKey, l)
}

func FromContext(ctx context.Context) Logger {
	maybeLogger := ctx.Value(loggerKey)
	if logger, ok := maybeLogger.(Logger); ok {
		return logger
	}
	return nil
}

func RequestWithLogger(req *http.Request, l Logger) *http.Request {
	return req.WithContext(ContextWithLogger(
		req.Context(),
		l.With(
			"uniqueId", strconv.FormatUint(rand.Uint64(), 16),
			"method", req.Method,
			"url", req.URL,
			"remote", req.RemoteAddr,
		),
	))
}

func FromRequest(req *http.Request) Logger {
	return FromContext(req.Context())
}

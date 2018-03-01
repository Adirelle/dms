package http

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"

	"github.com/anacrolix/dms/logging"
)

type contextKey int

const (
	uniqueIDKey = contextKey(1)
)

// UniqueID adds a unique ID to the Request Context, ResponseWriter and any associated Logger
func UniqueID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uniqueID := fmt.Sprintf("%08X", rand.Uint64())
		w.Header().Set("X-UniqueID", uniqueID)
		ctx := r.Context()
		if logger := logging.FromContext(ctx); logger != nil {
			ctx = logging.WithLogger(ctx, logger.With("uniqueID", uniqueID))
		}
		ctx = context.WithValue(ctx, uniqueIDKey, uniqueID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// UniqueIDFromContext retrieves the uniqueID from the Context
func UniqueIDFromContext(ctx context.Context) string {
	return ctx.Value(uniqueIDKey).(string)
}

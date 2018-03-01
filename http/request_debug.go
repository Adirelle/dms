package http

import (
	"fmt"
	"net/http"
	"time"

	"github.com/anacrolix/dms/logging"
)

// DebugRequest logs request start, status to its associated logger, if any
func DebugRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		drw := &debugResponseWriter{w: w, l: logging.MustFromContext(r.Context())}
		drw.Starts(r)
		defer drw.Ends(r)
		next.ServeHTTP(drw, r)
	})
}

type debugResponseWriter struct {
	w       http.ResponseWriter
	l       logging.Logger
	size    int
	started time.Time
	status  int
}

func (d *debugResponseWriter) Starts(r *http.Request) {
	d.started = time.Now()
	args := []interface{}{
		"remote", r.RemoteAddr,
		"host", r.Host,
		"method", r.Method,
		"url", r.URL,
	}
	if cType := r.Header.Get("Content-Type"); cType != "" {
		args = append(args, "content-type", cType)
	}
	d.l.Debugw("handling request", args...)
}

func (d *debugResponseWriter) Ends(r *http.Request) {
	args := []interface{}{
		"remote", r.RemoteAddr,
		"host", r.Host,
		"method", r.Method,
		"url", r.URL,
		"status", d.status,
		"elapsed", time.Since(d.started).String(),
		"content-length", d.size,
	}
	if cType := d.w.Header().Get("Content-Type"); cType != "" {
		args = append(args, "content-type", cType)
	}
	msg := fmt.Sprintf("request: %d %s", d.status, http.StatusText(d.status))
	if d.status < 100 || d.status >= 500 {
		d.l.Errorw(msg, args...)
	} else if d.status >= 400 {
		d.l.Infow(msg, args...)
	} else {
		d.l.Debugw(msg, args...)
	}
}

func (d *debugResponseWriter) Header() http.Header {
	return d.w.Header()
}

func (d *debugResponseWriter) Write(b []byte) (n int, err error) {
	d.WriteHeader(http.StatusOK)
	n, err = d.w.Write(b)
	d.size += n
	return
}

func (d *debugResponseWriter) WriteHeader(statusCode int) {
	if d.status != 0 {
		return
	}
	d.status = statusCode
	d.w.WriteHeader(statusCode)
}

func (d *debugResponseWriter) CloseNotify() <-chan bool {
	if cn, isCloseNotifier := d.w.(http.CloseNotifier); isCloseNotifier {
		return cn.CloseNotify()
	}
	return nil
}

func (d *debugResponseWriter) Flush() {
	if f, isFlusher := d.w.(http.Flusher); isFlusher {
		f.Flush()
	}
}

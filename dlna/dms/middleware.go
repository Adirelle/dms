package dms

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/anacrolix/dms/logging"
	"github.com/gorilla/handlers"
)

func SetHeadersHandler(headers map[string]string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			wh := w.Header()
			for k, v := range headers {
				wh.Set(k, v)
			}
			next.ServeHTTP(w, r)
		})
	}
}

func WithLoggerHandler(l logging.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, logging.RequestWithLogger(r, l))
		})
	}
}

func LogHeaderHandler() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			target := logging.FromRequest(r).Writer()
			defer target.Close()
			fmt.Fprintf(target, "%s %s %s\r\n", r.Method, r.URL.String(), r.Proto)
			r.Header.Write(target)
			io.Copy(target, r.Body)
			next.ServeHTTP(&headerLogger{w: w, target: target}, r)
		})
	}
}

type headerLogger struct {
	w             http.ResponseWriter
	target        io.Writer
	headerWritten bool
}

func (l *headerLogger) Header() http.Header {
	return l.w.Header()
}

func (l *headerLogger) Write(b []byte) (int, error) {
	l.WriteHeader(http.StatusOK)
	return l.w.Write(b)
}

func (l *headerLogger) WriteHeader(status int) {
	if l.headerWritten {
		return
	}
	l.headerWritten = true
	fmt.Fprintf(l.target, "HTTP/1.1 %d %s\r\n", status, http.StatusText(status))
	l.w.Header().Write(l.target)
	l.w.WriteHeader(status)
}

func (l *headerLogger) CloseNotify() <-chan bool {
	if cn, ok := l.w.(http.CloseNotifier); ok {
		return cn.CloseNotify()
	}
	return nil
}

func AccessLogHandler(w io.Writer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return handlers.CombinedLoggingHandler(w, next)
	}
}

func OpenAccessLogFile(path string) io.WriteCloser {
	if path == "-" {
		return nopWriterCloser{os.Stdout}
	}
	out, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
	if err != nil {
		log.Panicf("could not open access log file: %s", err.Error())
	}
	return out
}

type nopWriterCloser struct{ io.Writer }

func (nopWriterCloser) Close() error {
	return nil
}

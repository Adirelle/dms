package http

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
)

const (
	urlGeneratorKey = contextKey(2)
)

// URLSpec specifies how to build an URL with a route name and its parameters
type URLSpec struct {
	Route      string
	Parameters []string
}

// NewURLSpec is a helper to easily build an URLSPEC
func NewURLSpec(name string, pairs ...string) *URLSpec {
	return &URLSpec{name, pairs}
}

// URLGenerator generates a fully-fledged URL from the URLSpec
type URLGenerator interface {
	URL(*URLSpec) (string, error)
}

// RouterURLGenerator implements URLGenerator using a mux.Router
type RouterURLGenerator struct {
	router *mux.Router
	host   string
}

func (r *RouterURLGenerator) URL(s *URLSpec) (url string, err error) {
	route := r.router.Get(s.Route)
	if route == nil {
		return "", fmt.Errorf("unknown route %q", s.Route)
	}
	u, err := route.URLPath(s.Parameters...)
	if err != nil {
		return
	}
	u.Scheme = "http"
	u.Host = r.host
	url = u.String()
	return
}

// AddURLGenerator is a middleware that adds an URLGenerator in the Request Context
func AddURLGenerator(router *mux.Router) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r.WithContext(
				context.WithValue(r.Context(), urlGeneratorKey, &RouterURLGenerator{router, r.Host}),
			))
		})
	}
}

// URLGeneratorFromContext extracts the URLGenerator from the context
func URLGeneratorFromContext(ctx context.Context) URLGenerator {
	return ctx.Value(urlGeneratorKey).(URLGenerator)
}

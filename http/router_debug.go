package http

import (
	"fmt"
	"net/http"
	"reflect"

	"go.uber.org/zap/buffer"

	"github.com/gorilla/mux"
)

type RouterDebug struct{ *mux.Router }

type dumper struct {
	*buffer.Buffer
}

var bufferPool = buffer.NewPool()

func (d *RouterDebug) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	b := bufferPool.Get()
	defer b.Free()
	err := d.Walk(dumper{b}.dumpRoute)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", `text/plain; encoding="utf-8"`)
	w.Write(b.Bytes())
}

func (d dumper) dumpRoute(r *mux.Route, _ *mux.Router, _ []*mux.Route) error {
	fmt.Fprintln(d, "-")
	if name := r.GetName(); name != "" {
		fmt.Fprintf(d, "\tname: %s\n", r.GetName())
	}
	if err := r.GetError(); err != nil {
		fmt.Fprintf(d, "\terror: %s\n", err)
	}
	if v, err := r.GetHostTemplate(); err == nil {
		fmt.Fprintf(d, "\thostT: %s\n", v)
	}
	if v, err := r.GetMethods(); err == nil {
		fmt.Fprintf(d, "\tmethods: %s\n", v)
	}
	if v, err := r.GetPathTemplate(); err == nil {
		fmt.Fprintf(d, "\tpathT: %s\n", v)
	}
	if v, err := r.GetPathRegexp(); err == nil {
		fmt.Fprintf(d, "\tpathR: %s\n", v)
	}
	if v, err := r.GetQueriesTemplates(); err == nil {
		fmt.Fprintf(d, "\tqueryT: %s\n", v)
	}
	if v, err := r.GetQueriesRegexp(); err == nil {
		fmt.Fprintf(d, "\tqueryR: %s\n", v)
	}
	fmt.Fprintf(d, "\thandler: %v\n", reflect.ValueOf(r.GetHandler()).String())
	return nil
}

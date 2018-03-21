package rest

import (
	"bytes"
	"html/template"
	"net/http"
	"path"
	"strings"
	"time"

	"go.uber.org/zap/buffer"

	"github.com/Adirelle/dms/pkg/rest/templates"
	adi_http "github.com/Adirelle/go-libs/http"

	// go-bindata is used to generate templates/templates*.go
	_ "github.com/jteeuwen/go-bindata"
)

//go:generate go-bindata -o templates/templates.generated.go       -tags !debug -pkg templates -ignore .*\.go -prefix templates/ templates/...
//go:generate go-bindata -o templates/templates_debug.generated.go -tags debug  -pkg templates -ignore .*\.go -prefix templates/ templates/...

var bufferPool = buffer.NewPool()

type htmlProcessor struct{}

func (h htmlProcessor) CanProcess(mediaRange string) bool {
	return strings.EqualFold(mediaRange, "text/html") || strings.EqualFold(mediaRange, "application/xhtml+xml")
}

func (h htmlProcessor) Process(w http.ResponseWriter, req *http.Request, dataModel interface{}, context ...interface{}) (err error) {
	tpl, err := getTemplate()
	if err != nil {
		return
	}
	b := bufferPool.Get()
	defer b.Free()
	urlGen := adi_http.URLGeneratorFromContext(req.Context())
	err = tpl.Execute(b, map[string]interface{}{
		"model":        dataModel,
		"urlGenerator": urlGen,
		"url": func(name string, params ...string) string {
			url, err := urlGen.URL(adi_http.NewURLSpec(name, params...))
			if err != nil {
				panic(err)
			}
			return url
		},
	})
	if err != nil {
		return
	}

	w.Header().Set("Content-Type", `text/html; charset="utf-8"`)
	http.ServeContent(w, req, path.Base(req.URL.Path), time.Now(), bytes.NewReader(b.Bytes()))
	return
}

func buildTemplate() (*template.Template, error) {
	tplContent, err := templates.Asset("rest.tpl.html")
	if err != nil {
		return nil, err
	}
	return template.
		New("rest").
		Funcs(map[string]interface{}{
			"urlSpec": adi_http.NewURLSpec,
		}).
		Parse(string(tplContent))
}

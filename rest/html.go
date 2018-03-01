package rest

import (
	"bytes"
	"html/template"
	"net/http"
	"path"
	"strings"
	"time"

	"go.uber.org/zap/buffer"

	"github.com/anacrolix/dms/assets"
	dmsHttp "github.com/anacrolix/dms/http"
)

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
	urlGen := dmsHttp.URLGeneratorFromContext(req.Context())
	err = tpl.Execute(b, map[string]interface{}{
		"model":        dataModel,
		"urlGenerator": urlGen,
		"url": func(name string, params ...string) string {
			url, err := urlGen.URL(dmsHttp.NewURLSpec(name, params...))
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
	tplContent, err := assets.Asset("templates/rest.tpl.html")
	if err != nil {
		return nil, err
	}
	return template.
		New("rest").
		Funcs(map[string]interface{}{
			"urlSpec": dmsHttp.NewURLSpec,
		}).
		Parse(string(tplContent))
}

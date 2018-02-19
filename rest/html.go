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
)

var bufferPool = buffer.NewPool()

type htmlProcessor struct {
	Prefix string
}

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

	err = tpl.Execute(b, map[string]interface{}{
		"model":      dataModel,
		"pathPrefix": h.Prefix,
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
		Parse(string(tplContent))
}

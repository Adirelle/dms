package rest

import (
	"html/template"
	"net/http"
	"strings"

	"github.com/anacrolix/dms/assets"
)

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
	w.Header().Set("Content-Type", `text/html; charset="UTF-8"`)
	return tpl.Execute(w, map[string]interface{}{
		"model":      dataModel,
		"pathPrefix": h.Prefix,
	})
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

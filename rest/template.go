// +build !debug

package rest

import "html/template"

var tpl = template.Must(buildTemplate())

func (h htmlProcessor) getTemplate() (*template.Template, error) {
	return tpl, nil
}

// +build !debug

package rest

import "html/template"

var tpl = template.Must(buildTemplate())

func getTemplate() (*template.Template, error) {
	return tpl, nil
}

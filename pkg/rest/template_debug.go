// +build debug

package rest

import "html/template"

//go:generate go-bindata -o templates/templates_debug.go -tags debug -pkg templates -ignore .*\.go -prefix templates/ templates/...

func getTemplate() (*template.Template, error) {
	return buildTemplate()
}

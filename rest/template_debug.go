// +build debug

package rest

import "html/template"

func getTemplate() (*template.Template, error) {
	return buildTemplate()
}

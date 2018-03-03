package processor

import (
	"context"

	"github.com/anacrolix/dms/cds"
	"github.com/anacrolix/dms/didl_lite"
	"github.com/anacrolix/dms/http"
)

const (
	IconRoute          = "processor_icons"
	RouteIconParameter = "icon"
	RouteIconTemplate  = "{icon:.*}"
)

type BasicIconProcessor struct{}

func (BasicIconProcessor) String() string {
	return "BasicIconProcessor"
}

func (b BasicIconProcessor) Process(obj *cds.Object, _ context.Context) {
	obj.Tags[didl_lite.TagIcon] = http.NewURLSpec(IconRoute, RouteIconParameter, b.guessIcon(obj))
}

func (b BasicIconProcessor) guessIcon(obj *cds.Object) (icon string) {
	if obj.IsContainer() {
		return "folder"
	}
	t := obj.MimeType.Type
	if t == "audio" || t == "video" || t == "image" || t == "text" {
		return t
	}
	return "file"
}

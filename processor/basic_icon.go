package processor

import (
	"github.com/anacrolix/dms/cds"
	"github.com/anacrolix/dms/didl_lite"
	"github.com/anacrolix/dms/http"
)

const (
	IconRoute          = "processor_icons"
	RouteIconParameter = "icon"
	RouteIconTemplate  = "{icon:.*}"
)

type BasicIconProcessor struct {
}

func (b BasicIconProcessor) Process(obj *cds.Object) {
	icon := b.guessIcon(obj)
	if icon == "" {
		icon = "file"
	}
	obj.Tags[didl_lite.TagIcon] = http.NewURLSpec(IconRoute, RouteIconParameter, icon)
}

func (v BasicIconProcessor) guessIcon(obj *cds.Object) (icon string) {
	if obj.IsContainer() {
		return "folder"
	}
	mType := obj.MimeType.Type
	switch mType {
	case "audio", "video", "image", "text":
		return mType
	default:
		return "file"
	}
}

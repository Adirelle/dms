package processor

import (
	"github.com/anacrolix/dms/cds"
)

type BasicIconProcessor struct {
}

func (b BasicIconProcessor) Process(obj *cds.Object) error {
	icon := b.guessIcon(obj)
	if icon == "" {
		icon = "file"
	}
	obj.Tags.Set("upnp:icon", "/icons/"+icon+"-16.png")
	return nil
}

func (v BasicIconProcessor) guessIcon(obj *cds.Object) (icon string) {
	if obj.IsContainer() {
		return "folder"
	}
	mType := obj.MimeType().Type
	switch mType {
	case "audio", "video", "image":
		return mType
	default:
		return "file"
	}
}

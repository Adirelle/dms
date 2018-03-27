package basic_icon

import (
	"context"
	"net/http"

	"github.com/Adirelle/dms/pkg/cds"
	"github.com/Adirelle/dms/pkg/processor/basic_icon/icons"
	adi_http "github.com/Adirelle/go-libs/http"
	assetfs "github.com/elazarl/go-bindata-assetfs"
)

//go:generate go-bindata -o icons/icons.generated.go -pkg icons -ignore .*\.go -nocompress -prefix icons/ icons/...

const (
	IconRoute          = "processor_icons"
	RouteIconParameter = "icon"
	RouteIconTemplate  = "{icon:.*}"
)

type Processor struct{}

func (Processor) String() string {
	return "Processor"
}

func (b Processor) Process(obj *cds.Object, _ context.Context) {
	obj.Icon = adi_http.NewURLSpec(IconRoute, RouteIconParameter, b.guessIcon(obj))
}

func (b Processor) guessIcon(obj *cds.Object) (icon string) {
	if obj.IsContainer() {
		return "folder"
	}
	t := obj.MimeType.Type
	if t == "audio" || t == "video" || t == "image" || t == "text" {
		return t
	}
	return "file"
}

func (b Processor) Handler() http.Handler {
	fs := &assetfs.AssetFS{icons.Asset, icons.AssetDir, icons.AssetInfo, ""}
	return http.FileServer(fs)
}

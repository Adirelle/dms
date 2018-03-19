package processor

import (
	"context"
	"net/http"

	"github.com/Adirelle/dms/pkg/cds"
	"github.com/Adirelle/dms/pkg/didl_lite"
	"github.com/Adirelle/dms/pkg/processor/icons"
	adi_http "github.com/Adirelle/go-libs/http"
	assetfs "github.com/elazarl/go-bindata-assetfs"

	// go-bindata is used to generate assets*.go
	_ "github.com/jteeuwen/go-bindata"
)

//go:generate go-bindata -o icons/icons.go -pkg icons -ignore .*\.go -nocompress -prefix icons/ icons/...

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
	obj.Tags[didl_lite.TagIcon] = adi_http.NewURLSpec(IconRoute, RouteIconParameter, b.guessIcon(obj))
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

func (b BasicIconProcessor) Handler(prefix string) http.Handler {
	fs := &assetfs.AssetFS{icons.Asset, icons.AssetDir, icons.AssetInfo, prefix}
	return http.FileServer(fs)
}

package processor

import (
	"context"
	"regexp"

	dms_cache "github.com/Adirelle/dms/pkg/cache"
	"github.com/Adirelle/dms/pkg/cds"
	"github.com/Adirelle/dms/pkg/didl_lite"
	"github.com/Adirelle/dms/pkg/filesystem"
	"github.com/Adirelle/go-libs/cache"
	adi_http "github.com/Adirelle/go-libs/http"
	"github.com/Adirelle/go-libs/logging"
)

var (
	coverRegex = regexp.MustCompile(`(?i)(?:cover|front|face|albumart(?:small|large)?)\.(png|jpe?g|gif)$`)
)

type AlbumArtProcessor struct {
	fs *filesystem.Filesystem
	c  cache.Cache
	l  logging.Logger
}

func NewAlbumArtProcessor(fs *filesystem.Filesystem, cf *dms_cache.Factory, logger logging.Logger) *AlbumArtProcessor {
	a := &AlbumArtProcessor{fs: fs, l: logger}
	a.c = cf.Create("AlbumArt", a.loader)
	return a
}

func (AlbumArtProcessor) String() string {
	return "AlbumArtProcessor"
}

func (a *AlbumArtProcessor) Process(obj *cds.Object, ctx context.Context) {
	var parentID filesystem.ID

	if obj.IsContainer() {
		parentID = obj.ID
	} else if obj.MimeType.Type == "audio" {
		parentID = obj.ParentID()
	} else {
		return
	}

	uri, err := a.c.Get(parentID)
	if uri != nil {
		obj.Tags[didl_lite.TagAlbumArtURI] = uri.(*adi_http.URLSpec)
	} else if err != nil {
		logging.MustFromContext(ctx).Named("album-art").Warn(err)
	}
}

func (a *AlbumArtProcessor) loader(key interface{}) (res interface{}, err error) {
	parentID := key.(filesystem.ID)
	a.l.Debugf("processing: %v", parentID)

	parent, err := a.fs.Get(parentID)
	if err != nil {
		a.l.Warnf("error getting parent %s: %s", parentID, err)
		return
	}

	childrenID := parent.GetChildrenID()
	a.l.Debugf("%d children", len(childrenID))
	for _, childID := range childrenID {
		if !coverRegex.MatchString(childID.BaseName()) {
			a.l.Debugf("ignoring: %s", childID)
			continue
		}
		res = cds.FileServerURLSpec(childID)
		a.l.Debugf("result: %v", res)
		return
	}

	a.l.Debugf("nothing found")
	return
}

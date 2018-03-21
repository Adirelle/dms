package processor

import (
	"context"
	"regexp"

	dms_cache "github.com/Adirelle/dms/pkg/cache"
	"github.com/Adirelle/dms/pkg/cds"
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

func NewAlbumArtProcessor(fs *filesystem.Filesystem, cm *dms_cache.Manager, logger logging.Logger) *AlbumArtProcessor {
	a := &AlbumArtProcessor{fs: fs, l: logger}
	a.c = cm.Create("album-art", a.loader)
	return a
}

func (AlbumArtProcessor) String() string {
	return "AlbumArtProcessor"
}

func (a *AlbumArtProcessor) Process(obj *cds.Object, ctx context.Context) {
	parentID := obj.ID
	if !obj.IsContainer() {
		parentID = parentID.ParentID()
	}

	uri, err := a.c.Get(parentID)
	if uri != nil {
		obj.AlbumArtURI = uri.(*adi_http.URLSpec)
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

	a.l.Debugf("%d children", len(parent.ChildrenID))
	for _, childID := range parent.ChildrenID {
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

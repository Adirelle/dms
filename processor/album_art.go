package processor

import (
	"context"
	"regexp"

	"github.com/anacrolix/dms/cache"

	adi_http "github.com/Adirelle/go-libs/http"
	"github.com/Adirelle/go-libs/logging"
	"github.com/anacrolix/dms/cds"
	"github.com/anacrolix/dms/didl_lite"
	"github.com/anacrolix/dms/filesystem"
	"github.com/bluele/gcache"
)

var (
	coverRegex = regexp.MustCompile(`(?i)(?:cover|front|face|albumart(?:small|large)?)\.(png|jpe?g|gif)$`)
)

type AlbumArtProcessor struct {
	fs    *filesystem.Filesystem
	cache gcache.Cache
	l     logging.Logger
}

type artCacheKey struct {
	filesystem.ID
}

func NewAlbumArtProcessor(fs *filesystem.Filesystem, cache cache.MultiLoaderCache, logger logging.Logger) *AlbumArtProcessor {
	a := &AlbumArtProcessor{fs: fs, l: logger}
	var key artCacheKey
	a.cache = cache.RegisterLoaderFunc(&key, a.loader)
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

	uri, err := a.cache.Get(artCacheKey{parentID})
	if uri != nil {
		obj.Tags[didl_lite.TagAlbumArtURI] = uri.(*adi_http.URLSpec)
	} else if err != nil {
		logging.MustFromContext(ctx).Named("album-art").Warn(err)
	}
}

func (a *AlbumArtProcessor) loader(key interface{}) (res interface{}, err error) {
	parentID := key.(artCacheKey).ID
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

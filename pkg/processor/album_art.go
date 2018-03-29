package processor

import (
	"context"
	"encoding/gob"
	"regexp"

	"github.com/Adirelle/dms/pkg/cache"
	"github.com/Adirelle/dms/pkg/cds"
	"github.com/Adirelle/dms/pkg/filesystem"
	"github.com/Adirelle/go-libs/http"
	"github.com/Adirelle/go-libs/logging"
)

var (
	coverRegex = regexp.MustCompile(`(?i)(?:cover|front|face|albumart(?:small|large)?)\.(png|jpe?g|gif)$`)
)

type AlbumArtProcessor struct {
	fs *filesystem.Filesystem
	m  cache.Memo
	l  logging.Logger
}

type albumArt struct {
	filesystem.FileItem
	filesystem.ID
}

func init() {
	gob.Register(albumArt{})
}

func NewAlbumArtProcessor(fs *filesystem.Filesystem, cm *cache.Manager, logger logging.Logger) (a *AlbumArtProcessor) {
	a = &AlbumArtProcessor{fs: fs, l: logger}
	a.m = cm.NewMemo("album-art", albumArt{}, a.loader)
	return
}

func (AlbumArtProcessor) String() string {
	return "AlbumArtProcessor"
}

func (a *AlbumArtProcessor) Process(obj *cds.Object, ctx context.Context) {
	if coverRegex.MatchString(obj.Name) {
		return
	}

	parentID := obj.ID
	if !obj.IsContainer() {
		parentID = parentID.ParentID()
	}

	data := <-a.m.Get(parentID)
	if data == nil {
		return
	}
	aa := data.(*albumArt)
	if aa.ID.IsNull() {
		return
	}

	obj.AlbumArtURI = http.NewURLSpec(cds.FileServerRoute, cds.RouteObjectIDParameter, aa.ID.String())
}

func (a *AlbumArtProcessor) loader(key interface{}) (interface{}, error) {
	parentID := key.(filesystem.ID)
	a.l.Debugf("processing: %v", parentID)

	parent, err := a.fs.Get(parentID)
	if err != nil {
		a.l.Warnf("error getting parent %s: %s", parentID, err)
		return nil, err
	}

	aa := &albumArt{parent.FileItem, filesystem.NullID}

	a.l.Debugf("%d children", len(parent.ChildrenID))
	for _, childID := range parent.ChildrenID {
		if !coverRegex.MatchString(childID.BaseName()) {
			a.l.Debugf("ignoring: %s", childID)
			continue
		}
		aa.ID = childID
		break
	}

	a.l.Debugf("result: %v", aa.ID)
	return aa, nil
}

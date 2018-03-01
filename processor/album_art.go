package processor

import (
	"github.com/anacrolix/dms/cds"
	"github.com/anacrolix/dms/didl_lite"
	"github.com/anacrolix/dms/filesystem"
)

var coverNames = []string{"cover.jpg", "cover.png"}

type AlbumArtProcessor struct {
	Directory cds.ContentDirectory
}

func (a *AlbumArtProcessor) Process(obj *cds.Object) {
	var parentID filesystem.ID

	if obj.IsContainer() {
		parentID = obj.ID
	} else if obj.MimeType.Type == "audio" {
		parentID = obj.ParentID()
	} else {
		return
	}

	for _, name := range coverNames {
		cover, err := a.Directory.Get(parentID.ChildID(name))
		if err != nil {
			continue
		}
		obj.Tags[didl_lite.TagAlbumArtURI] = cds.FileServerURLSpec(cover)
		return
	}
}

package processor

import (
	"regexp"

	"github.com/anacrolix/dms/cds"
	"github.com/anacrolix/dms/didl_lite"
	"github.com/anacrolix/dms/filesystem"
)

var coverRegex = regexp.MustCompile(`(?i)(?:cover|front|face|albumart(?:small|large)?)\.(png|jpe?g|gif)$`)

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

	parent, err := a.Directory.Get(parentID)
	if err != nil {
		return
	}

	for _, childID := range parent.GetChildrenID() {
		if !coverRegex.MatchString(childID.BaseName()) {
			continue
		}
		cover, err := a.Directory.Get(childID)
		if err != nil || cover.MimeType.Type != "image" {
			continue
		}
		obj.Tags[didl_lite.TagAlbumArtURI] = cds.FileServerURLSpec(cover)
		return
	}
}

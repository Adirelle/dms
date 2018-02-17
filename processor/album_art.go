package processor

import (
	"github.com/anacrolix/dms/cds"
	"github.com/anacrolix/dms/logging"
)

const AlbumArtURI = "upnp:albumArtURI"

var coverNames = []string{"cover.jpg", "cover.png"}

type AlbumArtProcessor struct {
	Directory     cds.ContentDirectory
	PathGenerator cds.ObjectPathGenerator
	L             logging.Logger
}

func (a *AlbumArtProcessor) Process(obj *cds.Object) error {
	var parentID string
	if obj.IsContainer() {
		parentID = obj.ID
		a.L.Debugf("Looking for album art for container %s", obj.ID)
	} else if obj.MimeType().Type == "audio" {
		a.L.Debugf("Looking for album art for audio item %s", obj.ID)
		parentID = obj.ParentID
	} else {
		a.L.Debugf("Ignoring %s", obj.ID)
		return nil
	}
	for _, name := range coverNames {
		coverID := parentID + "/" + name
		cover, err := a.Directory.Get(coverID)
		if err != nil {
			continue
		}
		a.L.Debugf("Found %s", cover.ID)
		path := a.PathGenerator.URLPath(cover)
		obj.Tags.Set(AlbumArtURI, path)
	}
	return nil
}

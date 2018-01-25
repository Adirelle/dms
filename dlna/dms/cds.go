package dms

import (
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/anacrolix/dms/dlna"
	"github.com/anacrolix/dms/filesystem"
	"github.com/anacrolix/dms/misc"
	"github.com/anacrolix/dms/upnp"
	"github.com/anacrolix/dms/upnpav"
)

type contentDirectoryService struct {
	*Server
	upnp.Eventing
}

var updateID = fmt.Sprintf("%d", uint32(os.Getpid()))
var getSortCapabilities = map[string]string{"SortCaps": "dc:title"}
var getSearchCapabilities = map[string]string{"SearchCaps": ""}

func (cds *contentDirectoryService) Handle(action string, message []byte, r *http.Request) (map[string]string, error) {
	switch action {
	case "GetSystemUpdateID":
		return map[string]string{"Id": updateID}, nil
	case "GetSortCapabilities":
		return getSortCapabilities, nil
	case "GetSearchCapabilities":
		return getSearchCapabilities, nil
	case "Browse":
		return cds.Browse(message, r)
	default:
		return nil, upnp.InvalidActionError
	}
}

type browseQuery struct {
	ObjectID       string
	BrowseFlag     string
	Filter         string
	StartingIndex  int
	RequestedCount int

	req    *http.Request
	object filesystem.Object
}

type browseResponse struct {
	TotalMatches   int
	NumberReturned int
	Result         interface{}
	UpdateID       string
}

func (cds *contentDirectoryService) Browse(message []byte, r *http.Request) (resp *browseResponse, err error) {
	req := browseQuery{req: r}
	if err = xml.Unmarshal(message, &req); err != nil {
		return
	}
	if req.StartingIndex < 0 {
		err = upnp.Errorf(upnp.ArgumentValueInvalidErrorCode, "invalid StartingIndex: %d", req.StartingIndex)
		return
	}
	if req.RequestedCount < 0 {
		err = upnp.Errorf(upnp.ArgumentValueInvalidErrorCode, "invalid RequestedCount: %d", req.RequestedCount)
		return
	}
	req.object, err = cds.Filesystem.GetObjectByID(req.ObjectID)
	if err != nil {
		return
	}
	var items []interface{}
	switch req.BrowseFlag {
	case "BrowseDirectChildren":
		items, resp.TotalMatches, err = cds.BrowseDirectChildren(req)
	case "BrowseMetadata":
		items, resp.TotalMatches, err = cds.BrowseMetadata(req)
	default:
		return nil, upnp.Errorf(upnp.ArgumentValueInvalidErrorCode, "unhandled browse flag: %v", req.BrowseFlag)
	}
	if err != nil {
		return
	}
	xmlItems, err := xml.Marshal(items)
	if err == nil {
		resp.NumberReturned = len(items)
		resp.UpdateID = updateID
		resp.Result = didl_lite(string(xmlItems))
	}
	return
}

func (cds *contentDirectoryService) BrowseDirectChildren(req browseQuery) (items []interface{}, total int, err error) {
	children, err := req.object.Children()
	if err != nil {
		return
	}

	total = len(children)
	// TODO(anacrolix): Dig up why this special cast was added.
	sort.Sort(sortableObjects{children, strings.Contains(req.req.Header.Get("UserAgent"), `AwoX/1.1`)})

	if req.StartingIndex >= total {
		return
	} else if req.StartingIndex > 0 {
		children = children[req.StartingIndex:]
	}
	if req.RequestedCount > 0 && req.RequestedCount < len(children) {
		children = children[:req.RequestedCount]
	}

	items = make([]interface{}, len(children))
	for _, child := range children {
		item, itemErr := cds.GetObjectMetadata(child, req.req.Host)
		if itemErr == nil {
			items = append(items, item)
		} else {
			log.Printf("Ignored %s: %s", child.FilePath(), itemErr)
		}
	}
	return
}

func (cds *contentDirectoryService) BrowseMetadata(req browseQuery) (items []interface{}, total int, err error) {
	item, err := cds.GetObjectMetadata(req.object, req.req.Host)
	if err == nil {
		items = []interface{}{item}
		total = 1
	}
	return
}

// Turns the given entry and DMS host into a UPnP object. A nil object is
// returned if the entry is not of interest.
func (cds *contentDirectoryService) GetObjectMetadata(obj filesystem.Object, host string) (interface{}, error) {
	if obj.IsDir() {
		return cds.GetDirectoryMetadata(obj, host)
	}
	return cds.GetFileMetadata(obj, host)
}

func (cds *contentDirectoryService) GetDirectoryMetadata(dir filesystem.Object, host string) (interface{}, error) {
	return upnpav.Container{
		Object: upnpav.Object{
			ID:         dir.ID(),
			Class:      "object.container.storageFolder",
			ParentID:   dir.ParentID(),
			Restricted: 1,
			Title:      dir.Name(),
		},
	}, nil
}

func (cds *contentDirectoryService) GetFileMetadata(file filesystem.Object, host string) (ret interface{}, err error) {
	mimeType, err := MimeTypeByPath(file.FilePath())
	if err != nil {
		return
	}
	if !mimeType.IsMedia() {
		log.Printf("%s ignored: non-media file (%s)", file.FilePath(), mimeType)
		return
	}
	iconURI := (&url.URL{
		Scheme:   "http",
		Host:     host,
		Path:     iconPath,
		RawQuery: url.Values{"path": {file.ID()}}.Encode(),
	}).String()
	obj := upnpav.Object{
		ID:         file.ID(),
		Restricted: 1,
		ParentID:   file.ParentID(),
		Class:      "object.item." + mimeType.Type() + "Item",
		Title:      file.Name(),
		Icon:       iconURI,
		// TODO(anacrolix): This might not be necessary due to item res image element.
		AlbumArtURI: iconURI,
	}
	var (
		nativeBitrate uint
		resDuration   string
		resolution    string
	)
	if ffInfo, err := cds.FFProber.Probe(file.FilePath()); err != nil {
		log.Printf("error probing %s: %s", file.FilePath(), err)
	} else if ffInfo != nil {
		nativeBitrate, _ = ffInfo.Bitrate()
		if d, err := ffInfo.Duration(); err == nil {
			resDuration = misc.FormatDurationSexagesimal(d)
		}
		for _, strm := range ffInfo.Streams {
			if strm["codec_type"] == "video" {
				width := strm["width"]
				height := strm["height"]
				resolution = fmt.Sprintf("%.0fx%.0f", width, height)
				break
			}
		}
	}
	item := upnpav.Item{
		Object: obj,
		// Capacity: 1 for raw, 1 for icon, plus transcodes.
		Res: make([]upnpav.Resource, 0, 2+len(transcodes)),
	}
	item.Res = append(item.Res, upnpav.Resource{
		URL: (&url.URL{
			Scheme:   "http",
			Host:     host,
			Path:     resPath,
			RawQuery: url.Values{"path": {file.ID()}}.Encode(),
		}).String(),
		ProtocolInfo: fmt.Sprintf("http-get:*:%s:%s", mimeType, dlna.ContentFeatures{SupportRange: true}.String()),
		Bitrate:      nativeBitrate,
		Duration:     resDuration,
		Size:         uint64(file.Size()),
		Resolution:   resolution,
	})
	if mimeType.IsVideo() {
		if !cds.NoTranscode {
			item.Res = append(item.Res, transcodeResources(host, file.ID(), resolution, resDuration)...)
		}
	}
	if mimeType.IsVideo() || mimeType.IsImage() {
		item.Res = append(item.Res, upnpav.Resource{
			URL: (&url.URL{
				Scheme:   "http",
				Host:     host,
				Path:     iconPath,
				RawQuery: url.Values{"path": {file.ID()}, "c": {"jpeg"}}.Encode(),
			}).String(),
			ProtocolInfo: "http-get:*:image/jpeg:DLNA.ORG_PN=JPEG_TN",
		})
	}
	ret = item
	return
}

type sortableObjects struct {
	objets      []filesystem.Object
	foldersLast bool
}

func (so sortableObjects) Len() int {
	return len(so.objets)
}

func (so sortableObjects) Less(i, j int) bool {
	iIsDir := so.objets[i].IsDir()
	jIsDir := so.objets[j].IsDir()
	if iIsDir == jIsDir {
		return strings.ToLower(so.objets[i].Name()) < strings.ToLower(so.objets[j].Name())
	}
	less := iIsDir && !jIsDir
	if so.foldersLast {
		return !less
	}
	return less
}

func (so sortableObjects) Swap(i, j int) {
	so.objets[i], so.objets[j] = so.objets[j], so.objets[i]
}

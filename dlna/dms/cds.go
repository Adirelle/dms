package dms

import (
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/anacrolix/dms/soap"

	"github.com/anacrolix/dms/dlna"
	"github.com/anacrolix/dms/misc"
	"github.com/anacrolix/dms/upnp"
	"github.com/anacrolix/dms/upnpav"
)

type contentDirectoryService struct {
	*Server
	upnp.Eventing
}

type empty struct{}

func (cds *contentDirectoryService) updateIDString() string {
	return fmt.Sprintf("%d", uint32(os.Getpid()))
}

func (cds *contentDirectoryService) RegisterTo(s *soap.Server) {
	s.RegisterAction(soap.ActionFunc("urn:schemas-upnp-org:service:ContentDirectory:1#GetSystemUpdateID", cds.GetSystemUpdateID))
	s.RegisterAction(soap.ActionFunc("urn:schemas-upnp-org:service:ContentDirectory:1#GetSortCapabilities", cds.GetSortCapabilities))
	s.RegisterAction(soap.ActionFunc("urn:schemas-upnp-org:service:ContentDirectory:1#GetSearchCapabilities", cds.GetSearchCapabilities))
	s.RegisterAction(soap.ActionFunc("urn:schemas-upnp-org:service:ContentDirectory:1#Browse", cds.Browse))
}

// Turns the given entry and DMS host into a UPnP object. A nil object is
// returned if the entry is not of interest.
func (me *contentDirectoryService) cdsObjectToUpnpavObject(cdsObject object, fileInfo os.FileInfo, host, userAgent string) (ret upnpav.Object, err error) {
	entryFilePath := cdsObject.FilePath()
	ignored, err := me.IgnorePath(entryFilePath)
	if err != nil {
		return
	}
	if ignored {
		return
	}
	if fileInfo.IsDir() {
		return upnpav.NewContainer(
			cdsObject.ID(),
			cdsObject.ParentID(),
			upnpav.ObjectClassStorageFolder,
			fileInfo.Name(),
		), nil
	}
	if !fileInfo.Mode().IsRegular() {
		log.Printf("%s ignored: non-regular file", cdsObject.FilePath())
		return
	}
	mimeType, err := MimeTypeByPath(entryFilePath)
	if err != nil {
		return
	}
	if !mimeType.IsMedia() {
		log.Printf("%s ignored: non-media file (%s)", cdsObject.FilePath(), mimeType)
		return
	}

	ret = upnpav.NewItem(
		cdsObject.ID(),
		cdsObject.ParentID(),
		"object.item."+mimeType.Type()+"Item",
		fileInfo.Name(),
	)

	iconURI := (&url.URL{
		Scheme: "http",
		Host:   host,
		Path:   iconPath,
		RawQuery: url.Values{
			"path": {cdsObject.Path},
		}.Encode(),
	})
	ret.AddTag(upnpav.Tag{upnpav.TagNameIcon, iconURI.String()})
	ret.AddTag(upnpav.Tag{upnpav.TagNameAlbumArtURI, iconURI.String()})

	var (
		nativeBitrate uint
		resDuration   string
		resolution    string
	)
	if ffInfo, err := me.FFProber.Probe(entryFilePath); err != nil {
		log.Printf("error probing %s: %s", entryFilePath, err)
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

	ret.AddResource(upnpav.Resource{
		URL: (&url.URL{
			Scheme: "http",
			Host:   host,
			Path:   resPath,
			RawQuery: url.Values{
				"path": {cdsObject.Path},
			}.Encode(),
		}).String(),
		ProtocolInfo: fmt.Sprintf("http-get:*:%s:%s", mimeType, dlna.ContentFeatures{
			SupportRange: true,
		}.String()),
		Bitrate:    nativeBitrate,
		Duration:   resDuration,
		Size:       uint64(fileInfo.Size()),
		Resolution: resolution,
	})
	if mimeType.IsVideo() && !me.NoTranscode {
		for _, res := range transcodeResources(host, cdsObject.Path, resolution, resDuration) {
			ret.AddResource(res)
		}
	}
	if mimeType.IsVideo() || mimeType.IsImage() {
		ret.AddResource(upnpav.Resource{
			URL: (&url.URL{
				Scheme: "http",
				Host:   host,
				Path:   iconPath,
				RawQuery: url.Values{
					"path": {cdsObject.Path},
					"c":    {"jpeg"},
				}.Encode(),
			}).String(),
			ProtocolInfo: "http-get:*:image/jpeg:DLNA.ORG_PN=JPEG_TN",
		})
	}
	return
}

// Returns all the upnpav objects in a directory.
func (me *contentDirectoryService) readContainer(o object, host, userAgent string) (ret []upnpav.Object, err error) {
	sfis := sortableFileInfoSlice{
		// TODO(anacrolix): Dig up why this special cast was added.
		FoldersLast: strings.Contains(userAgent, `AwoX/1.1`),
	}
	sfis.fileInfoSlice, err = o.readDir()
	if err != nil {
		return
	}
	sort.Sort(sfis)
	for _, fi := range sfis.fileInfoSlice {
		child := object{path.Join(o.Path, fi.Name()), me.RootObjectPath}
		obj, err := me.cdsObjectToUpnpavObject(child, fi, host, userAgent)
		if err != nil {
			log.Printf("error with %s: %s", child.FilePath(), err)
			continue
		}
		if obj != nil {
			ret = append(ret, obj)
		}
	}
	return
}

// ContentDirectory object from ObjectID.
func (me *contentDirectoryService) objectFromID(id string) (o object, err error) {
	o.Path, err = url.QueryUnescape(id)
	if err != nil {
		return
	}
	if o.Path == "0" {
		o.Path = "/"
	}
	o.Path = path.Clean(o.Path)
	if !path.IsAbs(o.Path) {
		err = fmt.Errorf("bad ObjectID %v", o.Path)
		return
	}
	o.RootObjectPath = me.RootObjectPath
	return
}

type systemUpdateIDResponse struct {
	Name xml.Name `xml:"urn:schemas-upnp-org:service:ContentDirectory:1 GetSystemUpdateIDResponse"`
	Id   string
}

func (me *contentDirectoryService) GetSystemUpdateID(_ empty, _ *http.Request) (systemUpdateIDResponse, error) {
	return systemUpdateIDResponse{Id: me.updateIDString()}, nil
}

type getSortCapabilitiesResponse struct {
	Name     xml.Name `xml:"urn:schemas-upnp-org:service:ContentDirectory:1 GetSortCapabilitiesResponse"`
	SortCaps string
}

func (me *contentDirectoryService) GetSortCapabilities(_ empty, _ *http.Request) (getSortCapabilitiesResponse, error) {
	return getSortCapabilitiesResponse{SortCaps: "dc:title"}, nil
}

type getSearchCapabilitiesResponse struct {
	Name       xml.Name `xml:"urn:schemas-upnp-org:service:ContentDirectory:1 GetSearchCapabilitiesResponse"`
	SearchCaps string
}

func (me *contentDirectoryService) GetSearchCapabilities(_ empty, _ *http.Request) (getSearchCapabilitiesResponse, error) {
	return getSearchCapabilitiesResponse{SearchCaps: ""}, nil
}

type browseQuery struct {
	XMLName        xml.Name `xml:"urn:schemas-upnp-org:service:ContentDirectory:1 Browse"`
	ObjectID       string
	BrowseFlag     string
	Filter         string
	StartingIndex  int
	RequestedCount int
}

type browseReply struct {
	XMLName        xml.Name `xml:"urn:schemas-upnp-org:service:ContentDirectory:1 BrowseResponse"`
	TotalMatches   int
	NumberReturned int
	Result         browseReplyResults
}

type browseReplyResults struct {
	DIDLLite *upnpav.DIDLLite
}

func (me *contentDirectoryService) Browse(q browseQuery, req *http.Request) (rep browseReply, err error) {
	obj, err := me.objectFromID(q.ObjectID)
	if err != nil {
		return
	}
	var (
		total int
		objs  []upnpav.Object
	)
	switch q.BrowseFlag {
	case "BrowseDirectChildren":
		objs, total, err = me.BrowseDirectChildren(obj, q, req)
	case "BrowseMetadata":
		objs, total, err = me.BrowseMetadata(obj, req)
	default:
		err = upnp.Errorf(upnp.ArgumentValueInvalidErrorCode, "unhandled BrowseFlag: %q", q.BrowseFlag)
	}
	if err != nil {
		return
	}
	rep = browseReply{
		TotalMatches:   total,
		NumberReturned: len(objs),
		Result:         browseReplyResults{upnpav.NewDIDLLite(objs)},
	}
	return
}

func (me *contentDirectoryService) BrowseDirectChildren(obj object, q browseQuery, req *http.Request) (objs []upnpav.Object, total int, err error) {
	objs, err = me.readContainer(obj, req.Header.Get("Host"), req.Header.Get("User-Agent"))
	if err != nil {
		return
	}
	total = len(objs)
	if q.StartingIndex > 0 {
		if q.StartingIndex < total {
			objs = objs[q.StartingIndex:]
		} else {
			objs = objs[:0]
		}
	}
	if q.RequestedCount > 0 && q.RequestedCount < len(objs) {
		objs = objs[:q.RequestedCount]
	}
	return
}

func (me *contentDirectoryService) BrowseMetadata(obj object, req *http.Request) (objs []upnpav.Object, total int, err error) {
	fileInfo, err := os.Stat(obj.FilePath())
	if err != nil {
		if os.IsNotExist(err) {
			err = &upnp.Error{Code: upnpav.NoSuchObjectErrorCode, Desc: err.Error()}
		}
		return
	}
	upnp, err := me.cdsObjectToUpnpavObject(obj, fileInfo, req.Header.Get("Host"), req.Header.Get("User-Agent"))
	if err != nil {
		return
	}
	return []upnpav.Object{upnp}, 1, nil
}

// Represents a ContentDirectory object.
type object struct {
	Path           string // The cleaned, absolute path for the object relative to the server.
	RootObjectPath string
}

// Returns the number of children this object has, such as for a container.
func (cds *contentDirectoryService) objectChildCount(me object) int {
	objs, err := cds.readContainer(me, "", "")
	if err != nil {
		log.Printf("error reading container: %s", err)
	}
	return len(objs)
}

func (cds *contentDirectoryService) objectHasChildren(obj object) bool {
	return cds.objectChildCount(obj) != 0
}

// Returns the actual local filesystem path for the object.
func (o *object) FilePath() string {
	return filepath.Join(o.RootObjectPath, filepath.FromSlash(o.Path))
}

// Returns the ObjectID for the object. This is used in various ContentDirectory actions.
func (o object) ID() string {
	if !path.IsAbs(o.Path) {
		log.Panicf("Relative object path: %s", o.Path)
	}
	if len(o.Path) == 1 {
		return "0"
	}
	return url.QueryEscape(o.Path)
}

func (o *object) IsRoot() bool {
	return o.Path == "/"
}

// Returns the object's parent ObjectID. Fortunately it can be deduced from the
// ObjectID (for now).
func (o object) ParentID() string {
	if o.IsRoot() {
		return "-1"
	}
	o.Path = path.Dir(o.Path)
	return o.ID()
}

// This function exists rather than just calling os.(*File).Readdir because I
// want to stat(), not lstat() each entry.
func (o *object) readDir() (fis []os.FileInfo, err error) {
	dirPath := o.FilePath()
	dirFile, err := os.Open(dirPath)
	if err != nil {
		return
	}
	defer dirFile.Close()
	var dirContent []string
	dirContent, err = dirFile.Readdirnames(-1)
	if err != nil {
		return
	}
	fis = make([]os.FileInfo, 0, len(dirContent))
	for _, file := range dirContent {
		fi, err := os.Stat(filepath.Join(dirPath, file))
		if err != nil {
			continue
		}
		fis = append(fis, fi)
	}
	return
}

type sortableFileInfoSlice struct {
	fileInfoSlice []os.FileInfo
	FoldersLast   bool
}

func (me sortableFileInfoSlice) Len() int {
	return len(me.fileInfoSlice)
}

func (me sortableFileInfoSlice) Less(i, j int) bool {
	if me.fileInfoSlice[i].IsDir() && !me.fileInfoSlice[j].IsDir() {
		return !me.FoldersLast
	}
	if !me.fileInfoSlice[i].IsDir() && me.fileInfoSlice[j].IsDir() {
		return me.FoldersLast
	}
	return strings.ToLower(me.fileInfoSlice[i].Name()) < strings.ToLower(me.fileInfoSlice[j].Name())
}

func (me sortableFileInfoSlice) Swap(i, j int) {
	me.fileInfoSlice[i], me.fileInfoSlice[j] = me.fileInfoSlice[j], me.fileInfoSlice[i]
}

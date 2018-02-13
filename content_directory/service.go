package content_directory

import (
	"bytes"
	"encoding/xml"
	"net/http"

	"github.com/anacrolix/dms/filesystem"
	"github.com/anacrolix/dms/logging"
	"github.com/anacrolix/dms/upnp"
)

const (
	NoSuchObjectErrorCode = 701

	// Service identifier URN
	ServiceID = "urn:upnp-org:serviceId:ContentDirectory"

	// Service type URN
	ServiceType = "urn:schemas-upnp-org:service:ContentDirectory:1"
)

// Service implements the Content Directory Service
type Service struct {
	backend Backend
	fs      filesystem.Filesystem

	upnp   *upnp.Service
	logger logging.Logger
}

// New initializes a content-directory service
func NewService(backend Backend, fs filesystem.Filesystem, logger logging.Logger) *Service {
	s := &Service{
		backend: backend,
		fs:      fs,
		upnp:    upnp.NewService(ServiceID, ServiceType, logger),
		logger:  logger,
	}

	s.upnp.AddActionFunc("Browse", s.Browse)
	s.upnp.AddActionFunc("GetSystemUpdateID", s.GetSystemUpdateID)
	s.upnp.AddActionFunc("GetSortCapabilities", s.GetSortCapabilities)
	s.upnp.AddActionFunc("GetSearchCapabilities", s.GetSearchCapabilities)

	return s
}

func (s *Service) UPNPService() *upnp.Service {
	return s.upnp
}

func (s *Service) updateID() uint32 {
	root, err := s.fs.GetRootObject()
	if err != nil {
		s.logger.Panicf("could not fetch content-directory root: %s", err.Error())
	}
	return uint32(root.ModTime().Unix() & 0x7fff)
}

type empty struct{}

type systemUpdateIDResponse struct {
	XMLName xml.Name `xml:"u:GetSystemUpdateIDResponse"`
	XMLNS   string   `xml:"xmlns:u,attr"`
	ID      uint32   `xml:"Id" statevar:"SystemUpdateID"`
}

func (s *Service) GetSystemUpdateID(empty, *http.Request) (systemUpdateIDResponse, error) {
	return systemUpdateIDResponse{XMLNS: ServiceType, ID: s.updateID()}, nil
}

type getSortCapabilitiesResponse struct {
	XMLName  xml.Name `xml:"u:GetSortCapabilitiesResponse"`
	XMLNS    string   `xml:"xmlns:u,attr"`
	SortCaps string   `statevar:"SortCapabilities"`
}

func (s *Service) GetSortCapabilities(empty, *http.Request) (getSortCapabilitiesResponse, error) {
	return getSortCapabilitiesResponse{XMLNS: ServiceType, SortCaps: "dc:title"}, nil
}

type getSearchCapabilitiesResponse struct {
	XMLName    xml.Name `xml:"u:GetSearchCapabilitiesResponse"`
	XMLNS      string   `xml:"xmlns:u,attr"`
	SearchCaps string   `statevar:"SearchCapabilities"`
}

func (s *Service) GetSearchCapabilities(empty, *http.Request) (getSearchCapabilitiesResponse, error) {
	return getSearchCapabilitiesResponse{XMLNS: ServiceType, SearchCaps: ""}, nil
}

type browseQuery struct {
	XMLName        xml.Name `xml:"urn:schemas-upnp-org:service:ContentDirectory:1 Browse"`
	ObjectID       string   `statevar:"A_ARG_TYPE_ObjectID"`
	BrowseFlag     string   `statevar:"A_ARG_TYPE_BrowseFlag,string,BrowseMetadata,BrowseDirectChildren"`
	Filter         string   `statevar:"A_ARG_TYPE_Filter"`
	StartingIndex  uint32   `statevar:"A_ARG_TYPE_Index"`
	RequestedCount uint32   `statevar:"A_ARG_TYPE_Count"`
	SortCriteria   string   `statevar:"A_ARG_TYPE_SortCriteria"`
}

type browseReply struct {
	XMLName        xml.Name `xml:"u:BrowseResponse"`
	XMLNS          string   `xml:"xmlns:u,attr"`
	Result         []byte   `statevar:"A_ARG_TYPE_Result,string"`
	NumberReturned uint32   `statevar:"A_ARG_TYPE_Count"`
	TotalMatches   uint32   `statevar:"A_ARG_TYPE_Count"`
	UpdateID       uint32   `statevar:"A_ARG_TYPE_UpdateID"`
}

func (s *Service) Browse(q browseQuery, req *http.Request) (rep browseReply, err error) {
	obj, err := s.fs.GetObjectByID(q.ObjectID)
	if err != nil {
		return
	}
	var objs []Object
	switch q.BrowseFlag {
	case "BrowseDirectChildren":
		objs, rep.TotalMatches, err = s.browseDirectChildren(obj, q.StartingIndex, q.RequestedCount)
	case "BrowseMetadata":
		objs, rep.TotalMatches, err = s.browseMetadata(obj)
	default:
		err = upnp.Errorf(upnp.ArgumentValueInvalidErrorCode, "unhandled BrowseFlag: %q", q.BrowseFlag)
	}
	if err != nil {
		return
	}
	rep.XMLNS = ServiceType
	rep.NumberReturned = uint32(len(objs))
	rep.UpdateID = s.updateID()
	rep.Result, err = didlLite(objs)
	return
}

func didlLite(objs []Object) ([]byte, error) {
	b := bytes.Buffer{}
	_, err := b.WriteString(`<DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" xmlns:dlna="urn:schemas-dlna-org:device-1-0">`)
	if err == nil {
		err = xml.NewEncoder(&b).Encode(objs)
		if err == nil {
			_, err = b.WriteString(`</DIDL-Lite>`)
		}
	}
	return b.Bytes(), err
}

func (s *Service) browseDirectChildren(dir filesystem.Object, i uint32, n uint32) (objs []Object, total uint32, err error) {
	if !dir.IsDir() {
		return
	}
	children, err := dir.(filesystem.Directory).Children()
	if err != nil {
		return
	}
	total = uint32(len(children))
	if i > total {
		i = total
	}
	j := i + n
	if n == 0 || j > total {
		j = total
	}
	objs = make([]Object, 0, j-i)
	for ; i < j; i++ {
		if obj, err := s.backend.Get(children[i]); err == nil {
			objs = append(objs, obj)
		} else {
			s.logger.Warn(err)
		}
	}
	return
}

func (s *Service) browseMetadata(obj filesystem.Object) (objs []Object, total uint32, err error) {
	cdObj, err := s.backend.Get(obj)
	return []Object{cdObj}, 1, err
}

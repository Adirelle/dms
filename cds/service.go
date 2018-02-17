package cds

import (
	"bytes"
	"context"
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
	directory ContentDirectory

	upnp *upnp.Service
	l    logging.Logger
}

// New initializes a content-directory service
func NewService(directory ContentDirectory, logger logging.Logger) *Service {
	s := &Service{
		directory: directory,
		upnp:      upnp.NewService(ServiceID, ServiceType, logger),
		l:         logger,
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
	root, err := s.directory.Get(filesystem.RootID)
	if err != nil {
		s.l.Panicf("could not fetch content-directory root: %s", err.Error())
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
	Result         DIDLLite `statevar:"A_ARG_TYPE_Result,string"`
	NumberReturned uint32   `statevar:"A_ARG_TYPE_Count"`
	TotalMatches   uint32   `statevar:"A_ARG_TYPE_Count"`
	UpdateID       uint32   `statevar:"A_ARG_TYPE_UpdateID"`
}

func (s *Service) Browse(q browseQuery, req *http.Request) (r browseReply, err error) {
	if q.ObjectID == "0" {
		q.ObjectID = filesystem.RootID
	}
	ctx, cFunc := context.WithCancel(req.Context())
	defer cFunc()
	err = s.doBrowse(&r, q, ctx)
	if err != nil {
		return
	}
	r.XMLName.Space = q.XMLName.Space
	r.XMLNS = ServiceType
	r.UpdateID = s.updateID()
	r.NumberReturned = uint32(len(r.Result))
	return
}

func (s *Service) doBrowse(r *browseReply, q browseQuery, ctx context.Context) error {
	switch q.BrowseFlag {
	case "BrowseMetadata":
		return s.doBrowseMetadata(r, q, ctx)
	case "BrowseDirectChildren":
		return s.doBrowseDirectChildren(r, q, ctx)
	default:
		return upnp.Errorf(upnp.ArgumentValueInvalidErrorCode, "unhandled BrowseFlag: %q", q.BrowseFlag)
	}
}

func (s *Service) doBrowseMetadata(r *browseReply, q browseQuery, ctx context.Context) (err error) {
	obj, err := s.directory.Get(q.ObjectID)
	if err != nil {
		return
	}
	r.TotalMatches = 1
	r.Result.Append(obj)
	return
}

func (s *Service) doBrowseDirectChildren(r *browseReply, q browseQuery, ctx context.Context) (err error) {
	objs, errs := GetChildren(s.directory, q.ObjectID, ctx)
	open := true
	var obj *Object
	for open && err == nil {
		select {
		case _, open = <-ctx.Done():
			err = context.Canceled
		case obj, open = <-objs:
			if open {
				r.Result.Append(obj)
				r.TotalMatches++
			}
		case err = <-errs:
		}
	}
	if err != nil {
		return
	}
	SortObjects(r.Result)
	stoppingIndex := q.StartingIndex + q.RequestedCount
	if q.RequestedCount == 0 || stoppingIndex > r.TotalMatches {
		stoppingIndex = r.TotalMatches - q.StartingIndex
	}
	r.Result = r.Result[q.StartingIndex:stoppingIndex]
	return nil
}

// Slice of Objects, that are double-encoded
type DIDLLite []*Object

func (d *DIDLLite) Append(obj *Object) {
	*d = append(*d, obj)
}

func (d *DIDLLite) MarshalXML(e *xml.Encoder, start xml.StartElement) (err error) {
	b := bytes.Buffer{}
	_, err = b.WriteString(`<DIDL-Lite xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/" xmlns:dlna="urn:schemas-dlna-org:device-1-0">`)
	if err != nil {
		return
	}
	err = xml.NewEncoder(&b).Encode(d)
	if err != nil {
		return
	}
	_, err = b.WriteString(`</DIDL-Lite>`)
	if err != nil {
		return
	}
	return e.EncodeElement(xml.CharData(b.Bytes()), start)
}

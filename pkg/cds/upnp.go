package cds

import (
	"context"
	"encoding/xml"
	"net/http"

	"github.com/Adirelle/dms/pkg/didl_lite"
	"github.com/Adirelle/dms/pkg/filesystem"
	"github.com/Adirelle/dms/pkg/upnp"
	adi_http "github.com/Adirelle/go-libs/http"
	"github.com/Adirelle/go-libs/logging"
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
	ContentDirectory
	*upnp.Service
}

// New initializes a content-directory service
func NewService(directory ContentDirectory) *Service {
	s := &Service{
		directory,
		upnp.NewService(ServiceID, ServiceType),
	}

	s.AddActionFunc("Browse", s.Browse)
	s.AddActionFunc("GetSystemUpdateID", s.GetSystemUpdateID)
	s.AddActionFunc("GetSortCapabilities", s.GetSortCapabilities)
	s.AddActionFunc("GetSearchCapabilities", s.GetSearchCapabilities)

	return s
}

func (s *Service) updateID() uint32 {
	return uint32(s.LastModTime().Unix())
}

type empty struct {
	XMLName xml.Name
}

type systemUpdateIDResponse struct {
	XMLName xml.Name `xml:"u:GetSystemUpdateIDResponse"`
	XMLNS   string   `xml:"xmlns:u,attr"`
	ID      uint32   `xml:"Id" statevar:"SystemUpdateID"`
}

func (s *Service) GetSystemUpdateID(q empty, _ *http.Request) (systemUpdateIDResponse, error) {
	return systemUpdateIDResponse{XMLNS: q.XMLName.Space, ID: s.updateID()}, nil
}

type getSortCapabilitiesResponse struct {
	XMLName  xml.Name `xml:"u:GetSortCapabilitiesResponse"`
	XMLNS    string   `xml:"xmlns:u,attr"`
	SortCaps string   `statevar:"SortCapabilities"`
}

func (s *Service) GetSortCapabilities(q empty, _ *http.Request) (getSortCapabilitiesResponse, error) {
	return getSortCapabilitiesResponse{XMLNS: q.XMLName.Space, SortCaps: "dc:title"}, nil
}

type getSearchCapabilitiesResponse struct {
	XMLName    xml.Name `xml:"u:GetSearchCapabilitiesResponse"`
	XMLNS      string   `xml:"xmlns:u,attr"`
	SearchCaps string   `statevar:"SearchCapabilities"`
}

func (s *Service) GetSearchCapabilities(q empty, _ *http.Request) (getSearchCapabilitiesResponse, error) {
	return getSearchCapabilitiesResponse{XMLNS: q.XMLName.Space, SearchCaps: ""}, nil
}

type browseQuery struct {
	XMLName        xml.Name
	ObjectID       string `statevar:"A_ARG_TYPE_ObjectID"`
	BrowseFlag     string `statevar:"A_ARG_TYPE_BrowseFlag,string,BrowseMetadata,BrowseDirectChildren"`
	Filter         string `statevar:"A_ARG_TYPE_Filter"`
	StartingIndex  uint32 `statevar:"A_ARG_TYPE_Index"`
	RequestedCount uint32 `statevar:"A_ARG_TYPE_Count"`
	SortCriteria   string `statevar:"A_ARG_TYPE_SortCriteria"`
}

type browseReply struct {
	XMLName        xml.Name `xml:"u:BrowseResponse"`
	XMLNS          string   `xml:"xmlns:u,attr"`
	Result         []byte   `statevar:"A_ARG_TYPE_Result,string"`
	NumberReturned uint32   `statevar:"A_ARG_TYPE_Count"`
	TotalMatches   uint32   `statevar:"A_ARG_TYPE_Count"`
	UpdateID       uint32   `statevar:"A_ARG_TYPE_UpdateID"`
}

func (s *Service) Browse(q browseQuery, req *http.Request) (r browseReply, err error) {
	ctx, cFunc := context.WithCancel(req.Context())
	defer cFunc()
	var objs []*Object
	objs, r.TotalMatches, err = s.doBrowse(q, ctx)
	if err != nil {
		return
	}
	urlGen := adi_http.URLGeneratorFromContext(ctx)
	result := didl_lite.DIDLLite{}
	for _, o := range objs {
		if didl_obj, err := o.MarshalDIDLLite(urlGen); err == nil {
			result.AddObjects(didl_obj)
		} else {
			logging.MustFromContext(req.Context()).Warn(err)
		}
	}
	r.Result, err = xml.Marshal(result)
	if err != nil {
		return
	}
	r.NumberReturned = uint32(len(objs))
	r.XMLNS = q.XMLName.Space
	r.UpdateID = s.updateID()
	return
}

func (s *Service) doBrowse(q browseQuery, ctx context.Context) ([]*Object, uint32, error) {
	id, err := filesystem.ParseObjectID(q.ObjectID)
	if err != nil {
		return nil, 0, err
	}
	switch q.BrowseFlag {
	case "BrowseMetadata":
		return s.doBrowseMetadata(id, ctx)
	case "BrowseDirectChildren":
		return s.doBrowseDirectChildren(id, q.StartingIndex, q.RequestedCount, ctx)
	}
	return nil, 0, upnp.Errorf(upnp.ArgumentValueInvalidErrorCode, "unhandled BrowseFlag: %q", q.BrowseFlag)
}

func (s *Service) doBrowseMetadata(id filesystem.ID, ctx context.Context) (objs []*Object, total uint32, err error) {
	obj, err := s.Get(id, ctx)
	if err != nil {
		return
	}
	return []*Object{obj}, 1, nil
}

func (s *Service) doBrowseDirectChildren(id filesystem.ID, start uint32, limit uint32, ctx context.Context) (objs []*Object, total uint32, err error) {
	objs, err = s.GetChildren(id, ctx)
	if err != nil {
		return
	}
	total = uint32(len(objs))
	if start > total {
		start = total
	}
	end := start + limit
	if limit == 0 || end > total {
		end = total - start
	}
	objs = objs[start:end]
	return
}

package upnpav

import (
	"encoding/xml"
)

const (
	NoSuchObjectErrorCode = 701

	ObjectClassStorageFolder = "object.container.storageFolder"
	ObjectClassVideoItem
)

type Object interface {
	AddResource(...Resource)
	AddTag(...Tag)
}

type object struct {
	ID         string     `xml:"id,attr"`
	ParentID   string     `xml:"parentID,attr"`
	Restricted int        `xml:"restricted,attr"` // indicates whether the object is modifiable
	Title      string     `xml:"dc:title"`
	Class      string     `xml:"upnp:class"`
	Tags       []Tag      `xml:",any"`
	Res        []Resource `xml:"res,omitempty"`
}

func newObject(id, parentID, class, title string) *object {
	return &object{
		ID:         id,
		ParentID:   parentID,
		Restricted: 1,
		Class:      class,
		Title:      title,
	}
}

func (o *object) AddResource(res ...Resource) {
	o.Res = append(o.Res, res...)
}

func (o *object) AddTag(tags ...Tag) {
	o.Tags = append(o.Tags, tags...)
}

type Resource struct {
	XMLName      xml.Name `xml:"res"`
	ProtocolInfo string   `xml:"protocolInfo,attr"`
	URL          string   `xml:",chardata"`
	Size         uint64   `xml:"size,attr,omitempty"`
	Bitrate      uint     `xml:"bitrate,attr,omitempty"`
	Duration     string   `xml:"duration,attr,omitempty"`
	Resolution   string   `xml:"resolution,attr,omitempty"`
}

var (
	TagNameIcon        = xml.Name{Local: "upnp:icon"}
	TagNameArtist      = xml.Name{Local: "upnp:artist"}
	TagNameAlbum       = xml.Name{Local: "upnp:album"}
	TagNameGenre       = xml.Name{Local: "upnp:genre"}
	TagNameAlbumArtURI = xml.Name{Local: "upnp:albumArtURI"}
)

type Tag struct {
	XMLName xml.Name
	Value   string `xml:",chardata"`
}

type Container struct {
	XMLName xml.Name `xml:"container"`
	*object
	ChildCount int `xml:"childCount,attr,omitempty"`
}

func NewContainer(id, parentID, class, title string) Container {
	return Container{object: newObject(id, parentID, class, title)}
}

func (c *Container) SetChildCount(count int) {
	c.ChildCount = count
}

type Item struct {
	XMLName xml.Name `xml:"item"`
	*object
}

func NewItem(id, parentID, class, title string) Item {
	return Item{object: newObject(id, parentID, class, title)}
}

type DIDLLite struct {
	XMLName    xml.Name `xml:"urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/ DIDL-Lite"`
	XMLNS_DC   string   `xml:"xmlns:dc,attr"`
	XMLNS_UPNP string   `xml:"xmlns:upnp,attr"`
	XMLNS_DLNA string   `xml:"xmlns:dlna,attr"`
	Objects    []Object
}

func NewEmptyDIDLLite() *DIDLLite {
	return NewDIDLLite(nil)
}

func NewDIDLLite(objs []Object) *DIDLLite {
	return &DIDLLite{
		XMLNS_DC:   "http://purl.org/dc/elements/1.1/",
		XMLNS_UPNP: "urn:schemas-upnp-org:metadata-1-0/upnp/",
		XMLNS_DLNA: "urn:schemas-dlna-org:metadata-1-0/",
		Objects:    append(make([]Object, 0, len(objs)), objs...),
	}
}

func (d *DIDLLite) AddObject(obj ...Object) {
	d.Objects = append(d.Objects, obj...)
}

func (d *DIDLLite) NumObjects() int {
	return len(d.Objects)
}

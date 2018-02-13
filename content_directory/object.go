package content_directory

import (
	"encoding/xml"
)

type Object interface {
	AddResource(...Resource)
	AddTag(...Tag)
	GetTag(string) (string, bool)
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

func newObject(id, parentID, class, title string) object {
	return object{
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

func (o *object) GetTag(name string) (string, bool) {
	if name == "dc:title" {
		return o.Title, true
	}
	if name == "upnp:class" {
		return o.Class, true
	}
	for _, tag := range o.Tags {
		if tag.XMLName.Local == name {
			return tag.Value, true
		}
	}
	return "", false
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
	object
	ChildCount int `xml:"childCount,attr,omitempty"`
}

func NewContainer(id, parentID, class, title string) *Container {
	return &Container{object: newObject(id, parentID, class, title)}
}

func (c *Container) SetChildCount(count int) {
	c.ChildCount = count
}

type Item struct {
	XMLName xml.Name `xml:"item"`
	object
}

func NewItem(id, parentID, class, title string) *Item {
	return &Item{object: newObject(id, parentID, class, title)}
}

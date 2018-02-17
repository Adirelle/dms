package cds

import (
	"encoding/xml"
	"strings"

	"github.com/anacrolix/dms/filesystem"
	"github.com/h2non/filetype"
	types "gopkg.in/h2non/filetype.v1/types"
)

type Object struct {
	XMLName xml.Name
	filesystem.Object
	Restricted int        `xml:"restricted,attr"`
	Title      string     `xml:"dc:title"`
	Class      string     `xml:"upnp:class"`
	Res        []Resource `xml:"res,omitempty"`
	Tags       TagBag     `xml:",any,omitempty"`

	mimeType types.MIME
}

func newObject(obj *filesystem.Object) (o *Object, err error) {
	o = &Object{
		Object:     *obj,
		Restricted: 1,
		Tags:       TagBag(make(map[string]string)),
	}
	if o.IsContainer() {
		o.XMLName.Local = "container"
	} else {
		o.XMLName.Local = "item"
	}
	o.Title, o.mimeType, o.Class, err = guessMimeType(o)
	return
}

func guessMimeType(obj *Object) (title string, mimeType types.MIME, class string, err error) {
	if obj.IsContainer() {
		return obj.Name(), FolderType, "object.container", nil
	}
	typ, err := filetype.MatchFile(obj.FilePath)
	if err != nil {
		return
	}
	title = strings.TrimSuffix(obj.Name(), "."+typ.Extension)
	mimeType = typ.MIME
	if mimeType.Subtype == "audio" || mimeType.Subtype == "video" || mimeType.Subtype == "image" {
		class = "object." + mimeType.Subtype + "Item"
	} else {
		class = "object.item"
	}
	return
}

func (o *Object) AddResource(rs ...Resource) {
	for _, r := range rs {
		r.object = o
	}
	o.Res = append(o.Res, rs...)
}

func (o *Object) IsContainer() bool {
	return o.IsDir()
}

func (o *Object) MimeType() types.MIME {
	return o.mimeType
}

type Resource struct {
	XMLName      xml.Name   `xml:"res"`
	ProtocolInfo string     `xml:"protocolInfo,attr"`
	URL          string     `xml:",chardata"`
	Size         uint64     `xml:"size,attr,omitempty"`
	Bitrate      uint       `xml:"bitrate,attr,omitempty"`
	Duration     string     `xml:"duration,attr,omitempty"`
	Resolution   string     `xml:"resolution,attr,omitempty"`
	MimeType     types.MIME `xml:"-"`

	object *Object
}

func (r *Resource) Object() *Object {
	return r.object
}

type TagBag map[string]string

func (b *TagBag) Set(name, value string) {
	(*b)[name] = value
}

func (b *TagBag) Get(name string) (value string, found bool) {
	value, found = (*b)[name]
	return
}

func (b *TagBag) Remove(name string) {
	delete(*b, name)
}

func (b *TagBag) MarshalXML(e *xml.Encoder, _ xml.StartElement) error {
	for name, value := range *b {
		if err := e.EncodeElement(
			xml.CharData(value),
			xml.StartElement{Name: xml.Name{Local: name}},
		); err != nil {
			return err
		}
	}
	return nil
}
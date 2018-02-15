package cds

import (
	"encoding/xml"

	"github.com/anacrolix/dms/filesystem"
)

type Object struct {
	XMLName xml.Name
	filesystem.Object
	Restricted int        `xml:"restricted,attr"`
	Title      string     `xml:"dc:title"`
	Class      string     `xml:"upnp:class"`
	Res        []Resource `xml:"res,omitempty"`
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

func newObject(obj *filesystem.Object) (o *Object) {
	o = &Object{
		Object:     *obj,
		Restricted: 1,
		Title:      obj.Name(),
	}
	if o.IsDir() {
		o.XMLName.Local = "container"
	} else {
		o.XMLName.Local = "item"
	}
	o.Class = "object." + o.XMLName.Local
	return
}

func (o *Object) AddResource(res ...Resource) {
	o.Res = append(o.Res, res...)
}

func (o *Object) IsContainer() bool {
	return o.IsDir()
}

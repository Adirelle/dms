package didl_lite

import "encoding/xml"

// See http://upnp.org/schemas/av/didl-lite-v3.xsd for reference

// DIDLLite is the root element
type DIDLLite struct {
	XMLName xml.Name `xml:"urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/ DIDL-Lite"`
	Objects []Object `xml:"xmlns:,omitempty"`
}

var nsPrefixes = []xml.Attr{
	xml.Attr{xml.Name{Local: "xmlns:av"}, "urn:schemas-upnp-org:av:av"},
	xml.Attr{xml.Name{Local: "xmlns:dc"}, "http://purl.org/dc/elements/1.1/"},
	xml.Attr{xml.Name{Local: "xmlns:upnp"}, "urn:schemas-upnp-org:metadata-1-0/upnp/"},
}

func (d *DIDLLite) AddObjects(o ...Object) {
	d.Objects = append(d.Objects, o...)
}

func (d *DIDLLite) MarshalXML(e *xml.Encoder, el xml.StartElement) error {
	el.Attr = append(el.Attr, nsPrefixes...)
	return e.EncodeElement(d.Objects, el)
}

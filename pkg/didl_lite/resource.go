package didl_lite

import (
	"encoding/json"
	"encoding/xml"
)

const (
	ResDuration        = "duration"
	ResSize            = "size"
	ResBitrate         = "bitrate"
	ResSampleFrequency = "sampleFrequency"
	ResBitsPerSample   = "bitsPerSample"
	ResNrAudioChannels = "nrAudioChannels"
	ResResolution      = "resolution"
	ResColorDepth      = "colorDepth"
)

// Resource is a content to stream or download
type Resource struct {
	ID           string `xml:"id,attr,omitempty"`
	ProtocolInfo string `xml:"protocolInfo",attr"`
	URI          string `xml:",chardata"`

	// Optional attributs
	tags map[string]string
}

func (r *Resource) SetTag(name, value string) {
	if r.tags == nil {
		r.tags = make(map[string]string)
	}
	r.tags[name] = value
}

func (r *Resource) Tags() map[string]string {
	return r.tags
}

func (r *Resource) MarshalXML(e *xml.Encoder, el xml.StartElement) error {
	el.Attr = []xml.Attr{xml.Attr{xml.Name{Local: "protocolInfo"}, r.ProtocolInfo}}
	if r.ID != "" {
		el.Attr = append(el.Attr, xml.Attr{xml.Name{Local: "id"}, r.ID})
	}
	for name, value := range r.tags {
		el.Attr = append(el.Attr, xml.Attr{xml.Name{Local: name}, value})
	}
	return e.EncodeElement(r.URI, el)
}

func (r *Resource) MarshalJSON() ([]byte, error) {
	d := map[string]string{"uri": r.URI, "protocolInfo": r.ProtocolInfo}
	if r.ID != "" {
		d["id"] = r.ID
	}
	for name, value := range r.tags {
		d[name] = value
	}
	return json.Marshal(d)
}

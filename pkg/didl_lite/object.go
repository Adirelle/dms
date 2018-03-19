package didl_lite

import (
	"encoding/xml"
)

// See http://upnp.org/schemas/av/didl-lite-v3.xsd for reference

const (
	TagContributor          = "dc:contributor"
	TagCreator              = "dc:creator"
	TagDate                 = "dc:date"
	TagDescription          = "dc:description"
	TagLanguage             = "dc:language"
	TagPublisher            = "dc:publisher"
	TagRelation             = "dc:relation"
	TagRights               = "dc:rights"
	TagArtist               = "upnp:artist"
	TagActor                = "upnp:actor"
	TagAuthor               = "upnp:author"
	TagProducer             = "upnp:producer"
	TagDirector             = "upnp:director"
	TagGenre                = "upnp:genre"
	TagAlbum                = "upnp:album"
	TagAlbumArtURI          = "upnp:albumArtURI"
	TagArtistDiscographyURI = "upnp:artistDiscographyURI"
	TagLyricsURI            = "upnp:lyricsURI"
	TagLongDescription      = "upnp:longDescription"
	TagIcon                 = "upnp:icon"
	TagRegion               = "upnp:region"
	TagOriginalTrackNumber  = "upnp:originalTrackNumber"
	TagTOC                  = "upnp:toc"
	TagContainerUpdateID    = "upnp:containerUpdateID"
	TagObjectUpdateID       = "upnp:objectUpdateID"
)

// Object is either an Item or a Container
type Object interface {
	// Dummy private method to restrict the interface
	marker()
}

// Common holds the properties shared by Items and Containers
type Common struct {
	ID         string `xml:"id,attr"`
	ParentID   string `xml:"parentID,attr"`
	Restricted bool   `xml:"restricted,attr"`

	// Mandatory elements
	Title string `xml:"dc:title" json:"dc:title"`
	Class string `xml:"upnp:class" json:"upnp:class"`

	// Optional elements
	Tags tagElements

	// Related resources
	Resources []Resource `xml:"res" json:",omitempty"`
}

func (c *Common) marker() {}

func (c *Common) AddResources(r ...Resource) {
	c.Resources = append(c.Resources, r...)
}

type tagElements map[string]string

func (b *tagElements) Set(name, value string) {
	if *b == nil {
		*b = make(map[string]string)
	}
	(*b)[name] = value
}

func (b *tagElements) MarshalXML(e *xml.Encoder, _ xml.StartElement) error {
	if *b == nil {
		return nil
	}
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

// Container contains other Objects
type Container struct {
	XMLName             xml.Name `xml:"container" json:"-"`
	ChildCount          uint     `xml:"childCount,attr,omitempty" `
	ChildContainerCount uint     `xml:"childContainerCount,attr,omitempty" json:",omitempty"`
	Searchable          bool     `xml:"searchable,attr,omitempty" json:",omitempty"`

	Common

	Children []Object `json:",omitempty"`
}

func (c *Container) AddChildren(cs ...Object) {
	c.Children = append(c.Children, cs...)
}

func (Container) IsContainer() bool {
	return true
}

// Item is a group of related resources
type Item struct {
	XMLName xml.Name `xml:"item" json:"-"`
	Common
}

func (Item) IsContainer() bool {
	return false
}

package cds

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Adirelle/dms/pkg/didl_lite"
	"github.com/Adirelle/dms/pkg/filesystem"
	"github.com/Adirelle/go-libs/http"
	"github.com/h2non/filetype"
	types "gopkg.in/h2non/filetype.v1/types"
)

var FolderType = types.NewMIME("application/vnd.container")

type Object struct {
	filesystem.Object
	Title string

	Album       string
	AlbumArtURI *http.URLSpec
	Artist      string
	Date        time.Time
	Genre       string
	Icon        *http.URLSpec

	Resources []Resource

	MimeType types.MIME
}

func newObject(obj *filesystem.Object) (o *Object, err error) {
	o = &Object{Object: *obj}
	o.Title, o.MimeType, err = guessMimeType(o)
	return
}

func guessMimeType(obj *Object) (title string, mimeType types.MIME, err error) {
	if obj.IsContainer() {
		return obj.Name, FolderType, nil
	}
	typ, err := filetype.MatchFile(obj.FilePath)
	if err != nil {
		err = fmt.Errorf("error probing %q: %s", obj.FilePath, err.Error())
		return
	}
	title = strings.TrimSuffix(obj.Name, "."+typ.Extension)
	mimeType = typ.MIME
	return
}

func (o *Object) AddResource(rs ...Resource) {
	for _, r := range rs {
		r.Owner = o
	}
	o.Resources = append(o.Resources, rs...)
}

func (o *Object) IsContainer() bool {
	return o.IsDir
}

func (o *Object) MarshalDIDLLite(gen http.URLGenerator) (res didl_lite.Object, err error) {

	cm := didl_lite.Common{
		ID:         o.ID.String(),
		ParentID:   o.ID.ParentID().String(),
		Restricted: true,
		Title:      o.Title,
	}

	if !o.Date.IsZero() {
		cm.Tags.Set(didl_lite.TagDate, o.Date.Format(time.RFC3339))
	}
	if o.Artist != "" {
		cm.Tags.Set(didl_lite.TagArtist, o.Artist)
	}
	if o.Album != "" {
		cm.Tags.Set(didl_lite.TagAlbum, o.Album)
	}
	if o.Genre != "" {
		cm.Tags.Set(didl_lite.TagGenre, o.Genre)
	}

	var url string
	if o.Icon != nil {
		if url, err = gen.URL(o.Icon); err == nil {
			cm.Tags.Set(didl_lite.TagIcon, url)
		} else {
			return
		}
	}

	if o.AlbumArtURI != nil {
		if url, err = gen.URL(o.AlbumArtURI); err == nil {
			cm.Tags.Set(didl_lite.TagAlbumArtURI, url)
		} else {
			return
		}
	}

	for _, r := range o.Resources {
		var didlres didl_lite.Resource
		didlres, err = r.MarshalDIDLLite(gen)
		if err != nil {
			return
		}
		cm.AddResources(didlres)
	}

	if o.IsContainer() {
		cm.Class = "object.container"
		res = &didl_lite.Container{
			Common:     cm,
			ChildCount: uint(len(o.ChildrenID)),
		}
	} else {
		cm.Class = "object.item." + o.MimeType.Type + "Item"
		res = &didl_lite.Item{Common: cm}
	}

	return
}

type Resource struct {
	URL      *http.URLSpec
	Size     uint64
	MimeType types.MIME

	Duration        time.Duration
	Bitrate         uint32
	SampleFrequency uint32
	BitsPerSample   uint8
	NrAudioChannels uint8
	Resolution      didl_lite.Resolution
	ColorDepth      uint8

	FilePath string
	Owner    *Object
}

func (r *Resource) MarshalDIDLLite(gen http.URLGenerator) (res didl_lite.Resource, err error) {
	url, err := gen.URL(r.URL)
	if err != nil {
		return
	}
	res = didl_lite.Resource{
		ProtocolInfo: fmt.Sprintf("http-get:*:%s:*", r.MimeType.Value),
		URI:          url,
	}
	res.SetTag(didl_lite.ResSize, strconv.FormatUint(r.Size, 10))

	if r.Duration != 0 {
		res.SetTag(didl_lite.ResDuration, didl_lite.Duration(r.Duration).String())
	}
	if r.Bitrate != 0 {
		res.SetTag(didl_lite.ResBitrate, strconv.FormatUint(uint64(r.Bitrate), 10))
	}
	if r.SampleFrequency != 0 {
		res.SetTag(didl_lite.ResSampleFrequency, strconv.FormatUint(uint64(r.SampleFrequency), 10))
	}
	if r.BitsPerSample != 0 {
		res.SetTag(didl_lite.ResBitsPerSample, strconv.FormatUint(uint64(r.BitsPerSample), 10))
	}
	if r.NrAudioChannels != 0 {
		res.SetTag(didl_lite.ResNrAudioChannels, strconv.FormatUint(uint64(r.NrAudioChannels), 10))
	}
	if r.ColorDepth != 0 {
		res.SetTag(didl_lite.ResColorDepth, strconv.FormatUint(uint64(r.ColorDepth), 10))
	}
	if r.Resolution.Width != 0 && r.Resolution.Height != 0 {
		res.SetTag(didl_lite.ResResolution, r.Resolution.String())
	}

	return
}

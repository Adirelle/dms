package cds

import (
	"encoding"
	"fmt"
	"strconv"
	"strings"

	"github.com/Adirelle/dms/pkg/didl_lite"
	"github.com/Adirelle/dms/pkg/filesystem"
	"github.com/Adirelle/go-libs/http"
	"github.com/h2non/filetype"
	types "gopkg.in/h2non/filetype.v1/types"
)

var FolderType = types.NewMIME("application/vnd.container")

type Object struct {
	filesystem.Object
	Title     string
	Tags      map[string]interface{}
	Resources []Resource

	MimeType types.MIME
}

func newObject(obj *filesystem.Object) (o *Object, err error) {
	o = &Object{Object: *obj, Tags: make(map[string]interface{})}
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

	if o.Tags != nil {
		for name, value := range o.Tags {
			var repr string
			repr, err = marshalValue(value, gen)
			if err != nil {
				return
			}
			cm.Tags.Set(name, repr)
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
	FilePath string
	URL      *http.URLSpec
	Size     uint64
	Tags     map[string]interface{}
	MimeType types.MIME
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
	if r.Tags != nil {
		for name, value := range r.Tags {
			var repr string
			repr, err = marshalValue(value, gen)
			if err != nil {
				break
			}
			res.SetTag(name, repr)
		}
	}
	return
}

func (r *Resource) SetTag(name string, value interface{}) {
	if r.Tags == nil {
		r.Tags = make(map[string]interface{})
	}
	r.Tags[name] = value
}

func marshalValue(data interface{}, gen http.URLGenerator) (string, error) {
	switch value := data.(type) {
	case *http.URLSpec:
		if str, err := gen.URL(value); err == nil {
			return str, nil
		} else {
			return "", err
		}
	case encoding.TextMarshaler:
		if b, err := value.MarshalText(); err == nil {
			return string(b), nil
		} else {
			return "", err
		}
	case fmt.Stringer:
		return value.String(), nil
	case string:
		return value, nil
	case []byte:
		return string(value), nil
	}
	// switch reflect.ValueOf(data).Kind() {
	// case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
	// 	return fmt.Sprintf("%d", data), nil
	// }
	return fmt.Sprintf("%v", data), nil
}

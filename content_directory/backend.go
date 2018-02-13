package content_directory

import (
	"github.com/anacrolix/dms/logging"
	"github.com/gorilla/mux"

	"github.com/anacrolix/dms/filesystem"
)

// RouteRegisterer may be implements by TagProviders and ResourceProviders
type RouteRegisterer interface {
	RegisterRoutes(*mux.Router)
}

//
type Backend interface {
	RouteRegisterer
	AddTagProvider(TagProvider)
	AddResourceProvider(ResourceProvider)
	Get(filesystem.Object) (Object, error)
}

type backend struct {
	TagProviders      []TagProvider
	ResourceProviders []ResourceProvider

	logger logging.Logger
}

// TagProvider adds tags to Object
type TagProvider interface {
	ProvideTags(Object) error
}

// ResourceProvider adds resources to Object
type ResourceProvider interface {
	ProvideResources(Object) error
}

// NewSimpleBackend creates a simple content-directory Backend
func NewSimpleBackend(logger logging.Logger) Backend {
	return &backend{logger: logger}
}

// AddTagProvider registers a TagProvider to the ContentDirectory
func (b *backend) AddTagProvider(tp TagProvider) {
	b.TagProviders = append(b.TagProviders, tp)
}

// AddTagProvider registers a ResourceProvider to the ContentDirectory
func (b *backend) AddResourceProvider(rp ResourceProvider) {
	b.ResourceProviders = append(b.ResourceProviders, rp)
}

// RegisterRoutes registers the routes of all providers
func (b *backend) RegisterRoutes(router *mux.Router) {
	for _, tp := range b.TagProviders {
		if rr, ok := tp.(RouteRegisterer); ok {
			rr.RegisterRoutes(router)
		}
	}
	for _, rp := range b.ResourceProviders {
		if rr, ok := rp.(RouteRegisterer); ok {
			rr.RegisterRoutes(router)
		}
	}
}

// Get returns the CD object corresponding to the filesystem object
func (b *backend) Get(obj filesystem.Object) (Object, error) {
	if obj.IsDir() {
		return b.convertDirectory(obj.(filesystem.Directory))
	}
	return b.convertItem(obj)
}

func (b *backend) convertDirectory(obj filesystem.Directory) (Object, error) {
	dir := NewContainer(obj.ID(), obj.ParentID(), "object.container", obj.Name())
	dir.SetChildCount(obj.ChildrenCount())
	return dir, nil
}

func (b *backend) convertItem(obj filesystem.Object) (Object, error) {
	item := NewItem(obj.ID(), obj.ParentID(), "object.item", obj.Name())
	for _, tp := range b.TagProviders {
		if err := tp.ProvideTags(item); err != nil {
			b.logger.Warn(err)
		}
	}
	for _, rp := range b.ResourceProviders {
		if err := rp.ProvideResources(item); err != nil {
			b.logger.Warn(err)
		}
	}
	return item, nil
}

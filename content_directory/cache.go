package content_directory

import (
	"time"

	"github.com/anacrolix/dms/filesystem"
	"github.com/gorilla/mux"
)

type Cache interface {
	Get(key interface{}) (interface{}, bool)
	Put(key interface{}, value interface{})
	Del(key interface{})
}

type backendCache struct {
	inner Backend
	cache Cache
}

type cacheItem struct {
	object  Object
	modTime time.Time
}

func NewBackendCache(inner Backend, cache Cache) Backend {
	return &backendCache{inner, cache}
}

func (bc *backendCache) RegisterRoutes(r *mux.Router) {
	bc.inner.RegisterRoutes(r)
}

func (bc *backendCache) AddTagProvider(tp TagProvider) {
	bc.inner.AddTagProvider(tp)
}

func (bc *backendCache) AddResourceProvider(rp ResourceProvider) {
	bc.inner.AddResourceProvider(rp)
}

func (bc *backendCache) Get(obj filesystem.Object) (res Object, err error) {
	id := obj.ID()
	if cachedVal, found := bc.cache.Get(id); found {
		if item, ok := cachedVal.(cacheItem); ok && item.modTime.After(obj.ModTime()) {
			res = item.object
			return
		}
	}
	res, err = bc.inner.Get(obj)
	if err == nil {
		bc.cache.Put(id, cacheItem{res, obj.ModTime()})
	} else {
		bc.cache.Del(id)
	}
	return
}

package cds

import (
	"github.com/anacrolix/dms/logging"
	"github.com/bluele/gcache"
)

type Cache struct {
	directory ContentDirectory
	cache     gcache.Cache
	l         logging.Logger
}

func NewCache(directory ContentDirectory, cbuilder *gcache.CacheBuilder, logger logging.Logger) *Cache {
	c := &Cache{directory: directory, l: logger}
	c.cache = cbuilder.
		LoaderFunc(c.load).
		AddedFunc(c.added).
		EvictedFunc(c.evicted).
		Build()
	return c
}

func (c *Cache) Get(id string) (obj *Object, err error) {
	val, err := c.cache.Get(id)
	if err == nil {
		obj = val.(*Object)
	}
	return
}

func (c *Cache) load(key interface{}) (interface{}, error) {
	return c.directory.Get(key.(string))
}

func (c *Cache) added(key interface{}, _ interface{}) {
	c.l.Debugf("added %q", key)
}

func (c *Cache) evicted(key interface{}, _ interface{}) {
	c.l.Debugf("evicted %q", key)
}

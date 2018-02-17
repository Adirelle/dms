package cds

import (
	"time"

	"github.com/anacrolix/dms/logging"
	"github.com/bluele/gcache"
)

var SuccessTTL = time.Minute
var FailureTTL = 10 * time.Second

type Cache struct {
	directory ContentDirectory
	cache     gcache.Cache
	l         logging.Logger
}

func NewCache(directory ContentDirectory, cbuilder *gcache.CacheBuilder, logger logging.Logger) *Cache {
	c := &Cache{directory: directory, l: logger}
	c.cache = cbuilder.
		LoaderExpireFunc(c.load).
		AddedFunc(c.added).
		EvictedFunc(c.evicted).
		Build()
	return c
}

func (c *Cache) AddProcessor(priority int, p Processor) {
	c.directory.AddProcessor(priority, p)
}

func (c *Cache) Get(id string) (obj *Object, err error) {
	val, err := c.cache.Get(id)
	if err != nil {
		return
	}
	return val.(getResult).Resolve()
}

type getResult interface {
	Resolve() (*Object, error)
}

type getFailure struct{ err error }

func (f getFailure) Resolve() (*Object, error) {
	return nil, f.err
}

type getSuccess struct{ obj *Object }

func (s getSuccess) Resolve() (*Object, error) {
	return s.obj, nil
}

func (c *Cache) load(key interface{}) (interface{}, *time.Duration, error) {
	obj, err := c.directory.Get(key.(string))
	if err != nil {
		return getFailure{err}, &FailureTTL, nil
	}
	return getSuccess{obj}, &SuccessTTL, nil
}

func (c *Cache) added(key interface{}, _ interface{}) {
	c.l.Debugf("added %q", key)
}

func (c *Cache) evicted(key interface{}, _ interface{}) {
	c.l.Debugf("evicted %q", key)
}

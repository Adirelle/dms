package cds

import (
	"context"
	"time"

	"github.com/Adirelle/go-libs/logging"
	"github.com/anacrolix/dms/cache"
	"github.com/anacrolix/dms/filesystem"
	"github.com/bluele/gcache"
)

const LoaderTimeout = 5 * time.Second

type Cache struct {
	ContentDirectory
	cache gcache.Cache
	ctx   context.Context
}

func NewCache(d ContentDirectory, backend cache.MultiLoaderCache, l logging.Logger) *Cache {
	c := &Cache{
		ContentDirectory: d,
		ctx:              logging.WithLogger(context.Background(), l),
	}
	var key filesystem.ID
	c.cache = backend.RegisterLoaderFunc(&key, c.doGet)
	return c
}

func (c *Cache) Get(id filesystem.ID, ctx context.Context) (obj *Object, err error) {
	ch := make(chan struct{})
	go func() {
		defer close(ch)
		val, cerr := c.cache.Get(id)
		if val != nil && err == nil {
			obj = val.(*Object)
		} else {
			err = cerr
		}
	}()
	select {
	case <-ch:
	case <-ctx.Done():
		err = ctx.Err()
	}
	return
}

func (c *Cache) GetChildren(id filesystem.ID, ctx context.Context) (objs []*Object, err error) {
	return getChildren(c, id, ctx)
}

func (c *Cache) doGet(key interface{}) (obj interface{}, err error) {
	local, cancel := context.WithTimeout(c.ctx, LoaderTimeout)
	defer cancel()
	obj, err = c.ContentDirectory.Get(key.(filesystem.ID), local)
	return
}

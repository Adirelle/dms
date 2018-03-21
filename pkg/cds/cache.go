package cds

import (
	"context"
	"time"

	dms_cache "github.com/Adirelle/dms/pkg/cache"
	"github.com/Adirelle/dms/pkg/filesystem"
	"github.com/Adirelle/go-libs/cache"
	"github.com/Adirelle/go-libs/logging"
)

const LoaderTimeout = 5 * time.Second

type Cache struct {
	ContentDirectory
	c   cache.Cache
	ctx context.Context
}

func NewCache(d ContentDirectory, cm *dms_cache.Manager, l logging.Logger) *Cache {
	c := &Cache{
		ContentDirectory: d,
		ctx:              logging.WithLogger(context.Background(), l),
	}
	c.c = cm.Create("cds", c.loader)
	return c
}

func (c *Cache) Get(id filesystem.ID, ctx context.Context) (obj *Object, err error) {
	ch := make(chan struct{})
	go func() {
		defer close(ch)
		val, cerr := c.c.Get(id)
		if val != nil && cerr == nil {
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

func (c *Cache) loader(key interface{}) (obj interface{}, err error) {
	local, cancel := context.WithTimeout(c.ctx, LoaderTimeout)
	defer cancel()
	obj, err = c.ContentDirectory.Get(key.(filesystem.ID), local)
	return
}

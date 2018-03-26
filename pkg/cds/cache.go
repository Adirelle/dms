package cds

import (
	"context"
	"encoding/gob"
	"time"

	"github.com/Adirelle/dms/pkg/cache"
	"github.com/Adirelle/dms/pkg/filesystem"
	"github.com/Adirelle/go-libs/http"
	"github.com/Adirelle/go-libs/logging"
)

const LoaderTimeout = 5 * time.Second

func init() {
	gob.Register(Object{})
	gob.Register(http.URLSpec{})
}

type Cache struct {
	ContentDirectory
	m   cache.Memo
	ctx context.Context
}

func NewCache(d ContentDirectory, cm *cache.Manager, l logging.Logger) (*Cache, error) {
	c := &Cache{
		ContentDirectory: d,
		ctx:              logging.WithLogger(context.Background(), l),
	}
	var err error
	c.m, err = cm.NewMemo("cds", Object{}, c.loader)
	return c, err
}

func (c *Cache) Get(id filesystem.ID, ctx context.Context) (*Object, error) {
	select {
	case res := <-c.m.Get(id):
		return res.(*Object), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *Cache) GetChildren(id filesystem.ID, ctx context.Context) ([]*Object, error) {
	return getChildren(c, id, ctx)
}

func (c *Cache) loader(key interface{}) (obj interface{}, err error) {
	local, _ := context.WithTimeout(c.ctx, LoaderTimeout)
	obj, err = c.ContentDirectory.Get(key.(filesystem.ID), local)
	return
}

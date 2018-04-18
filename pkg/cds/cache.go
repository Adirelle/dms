package cds

import (
	"context"
	"encoding/gob"
	"fmt"
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

func NewCache(d ContentDirectory, cm *cache.Manager, l logging.Logger) *Cache {
	c := &Cache{
		ContentDirectory: d,
		ctx:              logging.WithLogger(context.Background(), l),
	}
	c.m = cm.NewMemo("cds", Object{}, c.loader)
	return c
}

func (c *Cache) Get(id filesystem.ID, ctx context.Context) (*Object, error) {
	select {
	case res, ok := <-c.m.Get(id):
		if ok {
			return res.(*Object), nil
		}
		return nil, fmt.Errorf("could not fetch %q", id)
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *Cache) GetChildren(id filesystem.ID, ctx context.Context) ([]*Object, error) {
	return getChildren(c, id, ctx)
}

func (c *Cache) loader(key interface{}) (interface{}, error) {
	local, cancel := context.WithTimeout(c.ctx, LoaderTimeout)
	defer cancel()
	return c.ContentDirectory.Get(key.(filesystem.ID), local)
}

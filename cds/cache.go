package cds

import (
	"context"
	"time"

	"github.com/Adirelle/go-libs/logging"
	"github.com/anacrolix/dms/cache"
	"github.com/anacrolix/dms/filesystem"
	"github.com/bluele/gcache"
)

const CacheLoaderTimeout = 5 * time.Second

type CacheConfig struct {
	Size       uint
	SuccessTTL time.Duration
	FailureTTL time.Duration
}

type Cache struct {
	CacheConfig
	ContentDirectory
	cache gcache.Cache
	ctx   context.Context
}

func NewCache(d ContentDirectory, conf CacheConfig, l logging.Logger) *Cache {
	c := &Cache{
		CacheConfig:      conf,
		ContentDirectory: d,
		ctx:              logging.WithLogger(context.Background(), l),
	}
	c.cache = cache.CacheErrors(
		gcache.New(int(conf.Size)).ARC(),
		c.doGet,
		conf.FailureTTL,
	)
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

func (c *Cache) doGet(key interface{}) (obj interface{}, ttl *time.Duration, err error) {
	local, cancel := context.WithTimeout(c.ctx, CacheLoaderTimeout)
	defer cancel()
	obj, err = c.ContentDirectory.Get(key.(filesystem.ID), local)
	if err == nil {
		ttl = &c.CacheConfig.SuccessTTL
	}
	return
}

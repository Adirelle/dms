package cds

import (
	"context"
	"time"

	"github.com/Adirelle/go-libs/logging"
	"github.com/anacrolix/dms/filesystem"
	"github.com/bluele/gcache"
)

const CacheLoaderTimeout = 5 * time.Second

type CacheConfig struct {
	Size       uint
	SuccessTTL time.Duration
	FailureTTL time.Duration
	// CacheSize          = 10000
	// CacheLoaderTimeout = 5 * time.Second
	// CacheSuccessTTL    = time.Minute
	// CacheFailureTTL    = 10 * time.Second
}

type Cache struct {
	CacheConfig
	ContentDirectory
	cache gcache.Cache
}

func NewCache(d ContentDirectory, conf CacheConfig, l logging.Logger) *Cache {
	ctx := logging.WithLogger(context.Background(), l)
	c := &Cache{
		CacheConfig:      conf,
		ContentDirectory: d,
		cache: gcache.
			New(int(conf.Size)).
			ARC().
			LoaderExpireFunc(wrapLoader(d.Get, ctx, &conf)).
			Build(),
	}
	return c
}

func (c *Cache) Get(id filesystem.ID, ctx context.Context) (obj *Object, err error) {
	ch := make(chan struct{})
	go func() {
		defer close(ch)
		obj, err = c.get(id)
	}()
	select {
	case <-ch:
	case <-ctx.Done():
		err = ctx.Err()
	}
	return
}

func (c *Cache) get(id filesystem.ID) (*Object, error) {
	val, err := c.cache.Get(id)
	if err != nil {
		return nil, err
	}
	return val.(getResult).Resolve()
}

func (c *Cache) GetChildren(id filesystem.ID, ctx context.Context) (objs []*Object, err error) {
	return getChildren(c, id, ctx)
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

func wrapLoader(loader func(filesystem.ID, context.Context) (*Object, error), ctx context.Context, conf *CacheConfig) func(interface{}) (interface{}, *time.Duration, error) {
	return func(key interface{}) (res interface{}, ttl *time.Duration, err error) {
		defer func() {
			if rec := logging.RecoverError(); rec != nil {
				res = getFailure{err}
				ttl = &conf.FailureTTL
			}
		}()
		local, cancel := context.WithTimeout(ctx, CacheLoaderTimeout)
		defer cancel()
		obj, err := loader(key.(filesystem.ID), local)
		if err != nil {
			res = getFailure{err}
			ttl = &conf.FailureTTL
		} else {
			res = getSuccess{obj}
			ttl = &conf.SuccessTTL
		}
		return
	}
}

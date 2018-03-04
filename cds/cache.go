package cds

import (
	"context"
	"time"

	"github.com/anacrolix/dms/filesystem"
	"github.com/Adirelle/go-libs/logging"
	"github.com/bluele/gcache"
)

var (
	CacheSize          = 10000
	CacheLoaderTimeout = 5 * time.Second
	CacheSuccessTTL    = time.Minute
	CacheFailureTTL    = 10 * time.Second
)

type Cache struct {
	ContentDirectory
	cache gcache.Cache
}

func NewCache(d ContentDirectory, l logging.Logger) *Cache {
	ctx := logging.WithLogger(context.Background(), l)
	c := &Cache{
		ContentDirectory: d,
		cache: gcache.
			New(CacheSize).
			ARC().
			LoaderExpireFunc(wrapLoader(d.Get, ctx)).
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
	TTL() *time.Duration
}

type getFailure struct{ err error }

func (getFailure) TTL() *time.Duration { return &CacheFailureTTL }

func (f getFailure) Resolve() (*Object, error) {
	return nil, f.err
}

type getSuccess struct{ obj *Object }

func (getSuccess) TTL() *time.Duration { return &CacheSuccessTTL }

func (s getSuccess) Resolve() (*Object, error) {
	return s.obj, nil
}

func wrapLoader(loader func(filesystem.ID, context.Context) (*Object, error), ctx context.Context) func(interface{}) (interface{}, *time.Duration, error) {
	return func(key interface{}) (res interface{}, ttl *time.Duration, err error) {
		var ret getResult
		defer func() {
			if rec := logging.RecoverError(); rec != nil {
				ret = getFailure{err}
			}
			res, ttl = ret, ret.TTL()
		}()
		local, cancel := context.WithTimeout(ctx, CacheLoaderTimeout)
		defer cancel()
		obj, err := loader(key.(filesystem.ID), local)
		if err != nil {
			ret = getFailure{err}
		} else {
			ret = getSuccess{obj}
		}
		err = nil
		return ret, ret.TTL(), nil
	}
}

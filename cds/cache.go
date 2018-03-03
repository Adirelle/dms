package cds

import (
	"context"
	"time"

	"github.com/anacrolix/dms/filesystem"
	"github.com/anacrolix/dms/logging"
	"github.com/bluele/gcache"
)

var (
	CacheLoaderTimeout = 5 * time.Second
	CacheSuccessTTL    = time.Minute
	CacheFailureTTL    = 10 * time.Second
)

type Cache struct {
	directory ContentDirectory
	cache     gcache.Cache
	l         logging.Logger
}

func NewCache(directory ContentDirectory, cbuilder *gcache.CacheBuilder, logger logging.Logger) *Cache {
	c := &Cache{directory: directory, l: logger}
	c.cache = cbuilder.LoaderExpireFunc(c.load).Build()
	return c
}

func (c *Cache) Get(id filesystem.ID, ctx context.Context) (obj *Object, err error) {
	ch := make(chan struct{})
	go func() {
		defer close(ch)
		obj, err = c.get(id).Resolve()
	}()
	select {
	case <-ch:
	case <-ctx.Done():
		err = ctx.Err()
	}
	return
}

func (c *Cache) get(id filesystem.ID) getResult {
	val, err := c.cache.Get(id)
	if err != nil {
		return getFailure{err}
	}
	return val.(getResult)
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

func (c *Cache) load(key interface{}) (res interface{}, ttl *time.Duration, err error) {
	var ret getResult
	defer func() {
		if rec := logging.RecoverError(); rec != nil {
			ret = getFailure{err}
		}
		res, ttl = ret, ret.TTL()
	}()
	ctx, _ := context.WithTimeout(
		logging.WithLogger(context.Background(), c.l),
		CacheLoaderTimeout,
	)
	obj, getErr := c.directory.Get(key.(filesystem.ID), ctx)
	if getErr != nil {
		ret = getFailure{getErr}
	} else {
		ret = getSuccess{obj}
	}
	return ret, ret.TTL(), nil
}

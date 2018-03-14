package cache

import (
	"time"

	"github.com/Adirelle/go-libs/logging"

	"github.com/bluele/gcache"
)

type failureCache struct {
	gcache.Cache
	loader     gcache.LoaderExpireFunc
	failureTTL time.Duration
}

/*
FailureCache creates a cache that caches errors and panics for a small amount of time.

This avoids burst of queries on a failing key.
*/
func FailureCache(builder *gcache.CacheBuilder, loader gcache.LoaderExpireFunc, failureTTL time.Duration) gcache.Cache {
	c := &failureCache{
		loader:     loader,
		failureTTL: failureTTL,
	}
	c.Cache = builder.LoaderExpireFunc(c.Load).Build()
	return c
}

func (c *failureCache) Load(key interface{}) (ret interface{}, ttl *time.Duration, err error) {
	defer func() {
		if rec := logging.RecoverError(); rec != nil {
			ret, ttl, err = rec, &c.failureTTL, nil
		}
	}()

	value, ttl, err := c.loader(key)
	if err != nil {
		return loaderFailure{err}, &c.failureTTL, nil
	}
	ret = loaderSuccess{value}
	return
}

func (c *failureCache) Get(key interface{}) (value interface{}, err error) {
	result, err := c.Cache.Get(key)
	if result != nil && err == nil {
		value, err = result.(loaderResult).Reveal()
	}
	return
}

func (c *failureCache) GetIFPresent(key interface{}) (value interface{}, err error) {
	result, err := c.Cache.GetIFPresent(key)
	if result != nil && err == nil {
		value, err = result.(loaderResult).Reveal()
	}
	return
}

type loaderResult interface {
	Reveal() (interface{}, error)
}

type loaderSuccess struct {
	value interface{}
}

func (s loaderSuccess) Reveal() (interface{}, error) {
	return s.value, nil
}

type loaderFailure struct {
	err error
}

func (f loaderFailure) Reveal() (interface{}, error) {
	return nil, f.err
}

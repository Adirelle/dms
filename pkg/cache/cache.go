package cache

import (
	"time"

	"github.com/Adirelle/go-libs/cache"
)

type Factory struct {
	Size int
	TTL  time.Duration
}

func (f *Factory) Create(name string, l cache.LoaderFunc) cache.Cache {
	c := cache.NewMemoryStorage(cache.Loader(l))
	if f.Size > 0 {
		c = cache.LRUEviction(f.Size)(c)
	}
	if f.TTL > 0 {
		c = cache.Expiration(f.TTL)(c)
	}
	return cache.SingleFlight(c)
}

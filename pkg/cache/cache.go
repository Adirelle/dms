package cache

import (
	"time"

	"github.com/Adirelle/go-libs/cache"
	"github.com/Adirelle/go-libs/logging"
	"github.com/boltdb/bolt"
)

type Manager struct {
	DB   *bolt.DB
	Size int
	TTL  time.Duration
	L    logging.Logger

	caches []cache.Cache
}

func (f *Manager) NewCache(name string, l cache.LoaderFunc, ff FactoryFunc) (c cache.Cache, err error) {
	mem := cache.NewMemoryStorage()
	if f.DB != nil {
		c, err = NewBoltDBStorage(f.DB, name, ff)
		if err != nil {
			return
		}
		c = cache.WriteThrough(mem)(c)
	} else {
		c = mem
	}
	if f.TTL > 0 {
		c = cache.Expiration(f.TTL)(c)
	}
	c = cache.Loader(l)(c)
	c = cache.SingleFlight(c)
	if f.Size > 0 {
		c = cache.LRUEviction(f.Size)(c)
	}
	c = cache.Name(name)(c)
	c = cache.Spy(f.L.Named(name).Debugf)(c)
	f.caches = append(f.caches, c)
	return
}

func (f *Manager) Flush() {
	f.L.Info("flushing")
	for _, c := range f.caches {
		c.Flush()
	}
}

package cache

import (
	"time"

	"github.com/Adirelle/go-libs/cache"
	"github.com/Adirelle/go-libs/logging"
	"github.com/boltdb/bolt"
	"github.com/ugorji/go/codec"
)

type Manager struct {
	DB   *bolt.DB
	Size int
	TTL  time.Duration
	L    logging.Logger
	H    codec.Handle

	caches []cache.Cache
}

func (f *Manager) Create(name string, l cache.LoaderFunc) cache.Cache {
	log := f.L.Named(name)
	c := cache.NewMemoryStorage(cache.Name(name))
	if f.Size > 0 {
		c = cache.LRUEviction(f.Size)(c)
	}
	if f.TTL > 0 {
		c = cache.Expiration(f.TTL)(c)
	}
	c = cache.Loader(l)(c)
	c = cache.SingleFlight(c)
	c = cache.Spy(log.Debugf)(c)
	f.caches = append(f.caches, c)
	return c
}

func (f *Manager) CreatePersistent(name string, l cache.LoaderFunc, ff FactoryFunc) (cache.Cache, error) {
	if f.DB == nil {
		return f.Create(name, l), nil
	}
	log := f.L.Named(name)
	c, err := NewBoltDBStorage(f.DB, name, NewCodecSerializer(ff, f.H), log)
	if err != nil {
		return nil, err
	}
	c = cache.Name(name)(c)
	mem := cache.NewMemoryStorage(cache.Name(name + "-wt"))
	if f.Size > 0 {
		c = cache.LRUEviction(f.Size * 2)(c)
		mem = cache.LRUEviction(f.Size)(mem)
	}
	c = cache.WriteThrough(mem)(c)
	if f.TTL > 0 {
		c = cache.Expiration(f.TTL)(c)
	}
	c = cache.Loader(l)(c)
	c = cache.SingleFlight(c)
	c = cache.Spy(log.Debugf)(c)
	f.caches = append(f.caches, c)
	return c, nil
}

func (f *Manager) Flush() {
	f.L.Info("flushing")
	for _, c := range f.caches {
		c.Flush()
	}
}

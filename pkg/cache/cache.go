package cache

import (
	"time"

	"gopkg.in/thejerf/suture.v2"

	"github.com/Adirelle/go-libs/cache"
	"github.com/boltdb/bolt"
)

type Manager struct {
	DB   *bolt.DB
	Size int
	TTL  time.Duration

	spv    *suture.Supervisor
	caches []cache.Cache
}

func (f *Manager) Create(l cache.LoaderFunc) cache.Cache {
	c := cache.NewMemoryStorage()
	if f.Size > 0 {
		c = cache.LRUEviction(f.Size)(c)
	}
	if f.TTL > 0 {
		c = cache.Expiration(f.TTL)(c)
	}
	c = cache.Loader(l)(c)
	c = cache.SingleFlight(c)
	f.caches = append(f.caches, c)
	return c
}

func (f *Manager) CreatePersistent(name string, l cache.LoaderFunc, s Serializer) cache.Cache {
	if f.DB == nil {
		return f.Create(l)
	}
	c := NewBoltDBStorage(f.DB, name, s)
	mem := cache.NewMemoryStorage()
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
	f.caches = append(f.caches, c)
	return c
}

func (f *Manager) Flush() {
	for _, c := range f.caches {
		c.Flush()
	}
}

func (f *Manager) Serve() {
	f.spv = suture.NewSimple("caches")
	for _, c := range f.caches {
		if svc, ok := c.(suture.Service); ok {
			f.spv.Add(svc)
		}
	}
	f.spv.Serve()
}

func (f *Manager) Stop() {
	f.spv.Stop()
	f.Flush()
}

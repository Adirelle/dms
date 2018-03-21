package cache

import (
	"time"

	"gopkg.in/thejerf/suture.v2"

	"github.com/Adirelle/go-libs/cache"
	"github.com/Adirelle/go-libs/logging"
	"github.com/boltdb/bolt"
)

type Manager struct {
	DB   *bolt.DB
	Size int
	TTL  time.Duration
	L    logging.Logger

	spv    *suture.Supervisor
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

func (f *Manager) CreatePersistent(name string, l cache.LoaderFunc, s Serializer) cache.Cache {
	if f.DB == nil {
		return f.Create(name, l)
	}
	c := NewBoltDBStorage(f.DB, name, s)
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
	f.caches = append(f.caches, c)
	return c
}

func (f *Manager) Flush() {
	f.L.Info("flushing")
	for _, c := range f.caches {
		c.Flush()
	}
}

func (f *Manager) Serve() {
	f.L.Info("serving")
	f.spv = suture.NewSimple("caches")
	for _, c := range f.caches {
		if svc, ok := c.(suture.Service); ok {
			f.spv.Add(svc)
		}
	}
	f.spv.Serve()
}

func (f *Manager) Stop() {
	f.L.Info("stopping")
	f.spv.Stop()
	f.Flush()
	f.L.Info("stopped")
}

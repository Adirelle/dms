package cache

import (
	"time"

	"github.com/bluele/gcache"
)

type MultiLoaderCache interface {
	RegisterLoaderFunc(interface{}, gcache.LoaderFunc) gcache.Cache
	RegisterLoaderExpireFunc(interface{}, gcache.LoaderExpireFunc) gcache.Cache
}

type Builder struct {
	builder    *gcache.CacheBuilder
	loader     gcache.LoaderExpireFunc
	failureTTL *time.Duration
	multi      MultiLoader

	cache gcache.Cache
}

func New(size int) *Builder {
	return &Builder{builder: gcache.New(size)}
}

func (b *Builder) FailureExpiraton(ttl time.Duration) *Builder {
	b.assertNotBuilt()
	b.failureTTL = &ttl
	return b
}

func (b *Builder) KeyLoaderFunc(key_sample interface{}, loader gcache.LoaderFunc) *Builder {
	return b.KeyLoaderExpireFunc(key_sample, wrapSimpleLoader(loader))
}

func (b *Builder) KeyLoaderExpireFunc(key_sample interface{}, loader gcache.LoaderExpireFunc) *Builder {
	if b.multi == nil {
		if b.loader != nil {
			panic("LoaderFunc already set")
		}
		b.multi = NewMultiLoader()
		b.loader = b.multi.Load
	}
	b.multi.RegisterLoaderExpire(key_sample, loader)
	return b
}

func (b *Builder) RegisterLoaderFunc(k interface{}, l gcache.LoaderFunc) gcache.Cache {
	return b.KeyLoaderFunc(k, l).Build()
}

func (b *Builder) RegisterLoaderExpireFunc(k interface{}, l gcache.LoaderExpireFunc) gcache.Cache {
	return b.KeyLoaderExpireFunc(k, l).Build()
}

func (b *Builder) Build() gcache.Cache {
	if b.cache == nil {
		if b.failureTTL != nil {
			if b.loader == nil {
				panic("A LoaderFunc is required to use FailureExpiraton")
			}
			b.cache = FailureCache(b.builder, b.loader, *b.failureTTL)
		} else {
			b.cache = b.builder.Build()
		}
	}
	return b.cache
}

func (b *Builder) assertNotBuilt() {
	if b.cache != nil {
		panic("Cache already cache")
	}
}

// Overidden gcache.CacheBuilder methods that return *Builder

func (b *Builder) Clock(clock gcache.Clock) *Builder {
	b.assertNotBuilt()
	b.builder.Clock(clock)
	return b
}

func (b *Builder) LoaderFunc(loaderFunc gcache.LoaderFunc) *Builder {
	b.assertNotBuilt()
	return b.LoaderExpireFunc(wrapSimpleLoader(loaderFunc))
}

func (b *Builder) LoaderExpireFunc(loaderExpireFunc gcache.LoaderExpireFunc) *Builder {
	b.assertNotBuilt()
	if b.multi != nil {
		panic("KeyLoaderFunc already set")
	}
	b.loader = loaderExpireFunc
	return b
}

func (b *Builder) EvictType(tp string) *Builder {
	b.assertNotBuilt()
	b.builder.EvictType(tp)
	return b
}

func (b *Builder) Simple() *Builder {
	b.assertNotBuilt()
	return b.EvictType(gcache.TYPE_SIMPLE)
}

func (b *Builder) LRU() *Builder {
	b.assertNotBuilt()
	return b.EvictType(gcache.TYPE_LRU)
}

func (b *Builder) LFU() *Builder {
	b.assertNotBuilt()
	return b.EvictType(gcache.TYPE_LFU)
}

func (b *Builder) ARC() *Builder {
	b.assertNotBuilt()
	return b.EvictType(gcache.TYPE_ARC)
}

func (b *Builder) EvictedFunc(evictedFunc gcache.EvictedFunc) *Builder {
	b.assertNotBuilt()
	b.builder.EvictedFunc(evictedFunc)
	return b
}

func (b *Builder) PurgeVisitorFunc(purgeVisitorFunc gcache.PurgeVisitorFunc) *Builder {
	b.assertNotBuilt()
	b.builder.PurgeVisitorFunc(purgeVisitorFunc)
	return b
}

func (b *Builder) AddedFunc(addedFunc gcache.AddedFunc) *Builder {
	b.assertNotBuilt()
	b.builder.AddedFunc(addedFunc)
	return b
}

func (b *Builder) DeserializeFunc(deserializeFunc gcache.DeserializeFunc) *Builder {
	b.assertNotBuilt()
	b.builder.DeserializeFunc(deserializeFunc)
	return b
}

func (b *Builder) SerializeFunc(serializeFunc gcache.SerializeFunc) *Builder {
	b.assertNotBuilt()
	b.builder.SerializeFunc(serializeFunc)
	return b
}

func (b *Builder) Expiration(expiration time.Duration) *Builder {
	b.assertNotBuilt()
	b.builder.Expiration(expiration)
	return b
}

// Helper

func wrapSimpleLoader(loaderFunc gcache.LoaderFunc) gcache.LoaderExpireFunc {
	return func(k interface{}) (v interface{}, ttl *time.Duration, err error) {
		v, err = loaderFunc(k)
		return
	}
}

type Config struct {
	Size       int
	Expiration time.Duration
	FailureTTL time.Duration
}

func (c Config) New() *Builder {
	b := New(c.Size)
	if c.Expiration > 0 {
		b.Expiration(c.Expiration)
	}
	if c.FailureTTL > 0 {
		b.FailureExpiraton(c.FailureTTL)
	}
	return b
}

package cache

import (
	"errors"
	"fmt"
	"sync"
)

// ErrKeyNotFound is returned by Cache.Get*() whenever the key is not present in the cache.
var ErrKeyNotFound = errors.New("Key not found")

// Cache is the main abstraction.
type Cache interface {
	// The string representation should be human-readable. It is used by Spy().
	fmt.Stringer

	// Put stores an entry into the cache.
	Put(key, value interface{}) error

	// Get fetchs an entry from the cache.
	// It returns nil and ErrKeyNotFound when the key is not present.
	Get(key interface{}) (value interface{}, err error)

	// Remove removes an entry from the cache.
	// It returns whether the entry was actually found and removed.
	Remove(key interface{}) bool

	// Flush instructs the cache to finish all pending operations, if any.
	// It must not return before all pending operations are finished.
	Flush() error

	// Len returns the number of entries in the cache.
	Len() int
}

// Option adds optional features new to a cache.
// Please note the order of options is important: they must be listed from outermost to innermost.
type Option func(Cache) Cache

type options []Option

func (o options) applyTo(c Cache) Cache {
	for i := len(o) - 1; i >= 0; i-- {
		c = o[i](c)
	}
	return c
}

// NewVoidStorage returns a cache that does not store nor return any entries, but can be used for side effects of options.
func NewVoidStorage(opts ...Option) Cache {
	return options(opts).applyTo(voidStorage{})
}

type voidStorage struct{}

func (voidStorage) Put(interface{}, interface{}) error   { return nil }
func (voidStorage) Get(interface{}) (interface{}, error) { return nil, ErrKeyNotFound }
func (voidStorage) Remove(interface{}) bool              { return false }
func (voidStorage) Flush() error                         { return nil }
func (voidStorage) Len() int                             { return 0 }
func (voidStorage) String() string                       { return "Void()" }

type namedCache struct {
	Cache
	name string
}

// Name gives a name to a cache. This name will be used by Spy(...).
func Name(name string) Option {
	return func(c Cache) Cache {
		return &namedCache{c, name}
	}
}

func (n *namedCache) String() string {
	return n.name
}

// NewMemoryStorage creates an empty cache using a map and a sync.RWMutex.
func NewMemoryStorage(opts ...Option) Cache {
	return options(opts).applyTo(&memoryStorage{items: make(map[interface{}]interface{})})
}

type memoryStorage struct {
	items map[interface{}]interface{}
	mu    sync.RWMutex
}

func (s *memoryStorage) Put(key interface{}, value interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[key] = value
	return nil
}

func (s *memoryStorage) Get(key interface{}) (interface{}, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if value, found := s.items[key]; found {
		return value, nil
	}
	return nil, ErrKeyNotFound
}

func (s *memoryStorage) Remove(key interface{}) (removed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, removed = s.items[key]; removed {
		delete(s.items, key)
	}
	return
}

func (s *memoryStorage) Flush() error {
	return nil
}

func (s *memoryStorage) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.items)
}

func (s *memoryStorage) String() string {
	return fmt.Sprintf("Memory(%p)", s.items)
}

type writeThrough struct {
	outer Cache
	inner Cache
	mu    sync.Mutex
}

// WriteThrough adds a second-level cache.
// Get operations are tried on "outer" first. If it fails, it tries the inner cache.
// If it succeed, the value is written to the outer cache.
// Put and remove operations are forwarded to both caches.
func WriteThrough(outer Cache) Option {
	return func(inner Cache) Cache {
		return &writeThrough{outer: outer, inner: inner}
	}
}

func (c *writeThrough) Put(key, value interface{}) (err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	err = c.inner.Put(key, value)
	if err == nil {
		err = c.outer.Put(key, value)
	}
	return
}

func (c *writeThrough) Get(key interface{}) (value interface{}, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	value, err = c.outer.Get(key)
	if err != ErrKeyNotFound {
		return
	}
	value, err = c.inner.Get(key)
	if err == nil {
		err = c.outer.Put(key, value)
	}
	return
}

func (c *writeThrough) Remove(key interface{}) (removed bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	removed = c.inner.Remove(key)
	return c.outer.Remove(key) || removed
}

func (c *writeThrough) Flush() (err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	err = c.inner.Flush()
	if err == nil {
		err = c.outer.Flush()
	}
	return
}

func (c *writeThrough) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Outer only contains a subset of entries of the inner cache.
	return c.inner.Len()
}

func (c *writeThrough) String() string {
	return fmt.Sprintf("WriteThrough(%s,%s)", c.outer, c.inner)
}

// LoaderFunc simulates a cache by calling the functions on call to Get.
type LoaderFunc func(interface{}) (interface{}, error)

type loader struct {
	Cache
	f LoaderFunc
}

// NewLoader creates a pseudo-cache from a LoaderFunc.
func NewLoader(f LoaderFunc, opts ...Option) Cache {
	return options(opts).applyTo(&loader{voidStorage{}, f})
}

// Loader adds a layer to generate values on demand.
func Loader(f LoaderFunc) Option {
	return func(c Cache) Cache {
		return &loader{c, f}
	}
}

func (l *loader) Get(key interface{}) (value interface{}, err error) {
	value, err = l.Cache.Get(key)
	if err != ErrKeyNotFound {
		return
	}
	value, err = l.f(key)
	if err == nil {
		err = l.Cache.Put(key, value)
	}
	return
}

func (l *loader) String() string {
	return fmt.Sprintf("Loader(%s,%v)", l.Cache, l.f)
}

// ValidatorFunc is used to validate cache entries.
type ValidatorFunc func(key, value interface{}) (bool, error)

type validator struct {
	Cache
	f ValidatorFunc
}

// Validate validates every entry using the given function.
func Validate(f ValidatorFunc) Option {
	return func(c Cache) Cache {
		return &validator{c, f}
	}
}

func (c *validator) String() string {
	return fmt.Sprintf("Validator(%s,%v)", c.Cache, c.f)
}

func (c *validator) Get(key interface{}) (value interface{}, err error) {
	value, err = c.Cache.Get(key)
	if err != nil {
		return
	}
	ok, err := c.f(key, value)
	if err == nil && !ok {
		err = ErrKeyNotFound
	}
	if err != nil {
		value = nil
		c.Cache.Remove(key)
	}
	return
}

// Validable can validate itself
type Validable interface {
	IsValid() (bool, error)
}

// ValidateValidable is a ValidatorFunc that handles Validable.
func ValidateValidable(key, value interface{}) (isValid bool, err error) {
	if v, ok := value.(Validable); ok {
		isValid, err = v.IsValid()
	}
	return
}

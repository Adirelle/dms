package cache

import (
	"encoding/gob"
	"fmt"
	"time"
)

type expiringCache struct {
	Cache
	Clock
	ttl time.Duration
}

type expirableItem struct {
	Value      interface{}
	Expiration time.Time
}

func init() {
	gob.Register(expirableItem{})
}

// Expiration adds automatic expiration to new entries using the given delay.
func Expiration(ttl time.Duration) Option {
	return ExpirationUsingClock(ttl, RealClock)
}

// ExpirationUsingClock adds automatic expiration to new entries using the given clock.
func ExpirationUsingClock(ttl time.Duration, cl Clock) Option {
	return func(c Cache) Cache {
		return &expiringCache{Cache: c, Clock: cl, ttl: ttl}
	}
}

func (e *expiringCache) Put(key, value interface{}) error {
	return e.PutWithTTL(key, value, e.ttl)
}

func (e *expiringCache) PutWithTTL(key, value interface{}, ttl time.Duration) error {
	return e.Cache.Put(key, &expirableItem{value, e.Now().Add(ttl)})
}

func (e *expiringCache) Get(key interface{}) (interface{}, error) {
	item, err := e.Cache.Get(key)
	if err != nil {
		return nil, err
	}
	it := item.(*expirableItem)
	if it.Expiration.Before(e.Now()) {
		e.Cache.Remove(key)
		return nil, ErrKeyNotFound
	}
	return it.Value, nil
}

func (e *expiringCache) String() string {
	return fmt.Sprintf("Expiring(%s,%s)", e.Cache, e.ttl)
}

// Clock is a simple clock abstraction to be used with ExpirationUsingClock.
type Clock interface {
	Now() time.Time
}

// RealClock is a Clock implementation that uses time.Now().
var RealClock Clock = realClock{}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

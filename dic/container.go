package dic

import (
	"fmt"
	"log"
	"reflect"
	"sync"

	"github.com/anacrolix/dms/logging"
)

type Container interface {
	Has(interface{}) bool
	Get(interface{}) (interface{}, error)
	get(...interface{}) (reflect.Value, error)
}

type BaseContainer struct {
	Builder
	instances map[Provider]*result
	lk        sync.Mutex
}

func New() *BaseContainer {
	c := &BaseContainer{
		Builder:   builder(make(map[interface{}]Provider)),
		instances: make(map[Provider]*result),
	}
	c.Register(Constant(c), "container")
	return c
}

func (c *BaseContainer) Has(key interface{}) (ok bool) {
	_, ok = c.ProviderFor(key)
	return
}

func (c *BaseContainer) Get(key interface{}) (value interface{}, err error) {
	v, err := c.get(key)
	if err == nil {
		value = v.Interface()
	}
	return
}

func (c *BaseContainer) get(keys ...interface{}) (value reflect.Value, err error) {
	p, err := c.findProviderFor(keys)
	if err != nil {
		return
	}
	c.lk.Lock()
	res, exists := c.instances[p]
	if !exists {
		res = deferred(func() (reflect.Value, error) { return c.Build(p, c) })
		c.instances[p] = res
	}
	c.lk.Unlock()
	return res.Await()
}

func (c *BaseContainer) findProviderFor(keys []interface{}) (Provider, error) {
	for _, k := range keys {
		if p, found := c.ProviderFor(k); found {
			return p, nil
		}
	}
	return nil, &UnknownError{keys}
}

type UnknownError struct {
	Keys []interface{}
}

func (e *UnknownError) Error() string {
	return fmt.Sprintf("do not know how to build %v", e.Keys)
}

type result struct {
	Value reflect.Value
	Err   error
	sync.RWMutex
}

func deferred(f func() (reflect.Value, error)) (r *result) {
	r = &result{}
	r.Lock()
	go func() {
		defer r.Unlock()
		r.Value, r.Err = f()
	}()
	return
}

func (r *result) Await() (reflect.Value, error) {
	r.RLock()
	defer r.RUnlock()
	return r.Value, r.Err
}

func (c *BaseContainer) RegisterConstants(pairs ...interface{}) {
	n := len(pairs)
	for i := 0; i < n; i += 2 {
		c.Register(Constant(pairs[i+1]), pairs[i])
	}
}

func (c *BaseContainer) RegisterAuto(values ...interface{}) {
	for _, v := range values {
		c.Register(Auto(v))
	}
}

func (c *BaseContainer) Mimic(struc interface{}) {
	v := reflect.ValueOf(struc)
	t := v.Type()
	if t.Kind() != reflect.Struct {
		log.Panicf("Mimic argument must be a Struct, not a %v", t.Kind())
	}
	for i := 0; i < t.NumField(); i++ {
		c.Register(Constant(v.Field(i).Interface()), t.Field(i).Name)
	}
	for i := 0; i < t.NumMethod(); i++ {
		c.Register(Func(v.Method(i).Interface()), t.Method(i).Name)
	}
}

func (c *BaseContainer) LogTo(l logging.Logger) {
	c.Builder = &loggingBuilder{Builder: c.Builder, L: l}
}

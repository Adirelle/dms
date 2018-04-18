package cache

import (
	"sync"
)

type SingleFlight struct {
	calls map[interface{}]*call
	mu    sync.Mutex
}

func NewSingleFlight() *SingleFlight {
	return &SingleFlight{calls: make(map[interface{}]*call)}
}

func (f *SingleFlight) Do(key interface{}, fn func(interface{}) (interface{}, bool)) <-chan interface{} {
	ch := make(chan interface{}, 1)
	c := f.getOrStart(key, fn)
	c.Listen(ch)
	return ch
}

func (f *SingleFlight) getOrStart(key interface{}, fn func(interface{}) (interface{}, bool)) *call {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := f.calls[key]
	if c == nil {
		c = new(call)
		f.calls[key] = c
		go func() {
			defer f.done(key)
			c.Run(func() (interface{}, bool) { return fn(key) })
		}()
	}
	return c
}

func (f *SingleFlight) done(key interface{}) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.calls, key)
}

type call struct {
	chs []chan<- interface{}
	mu  sync.Mutex
}

func (c *call) Listen(ch chan<- interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.chs = append(c.chs, ch)
}

func (c *call) Run(fn func() (interface{}, bool)) {
	defer c.close()
	if value, ok := fn(); ok {
		c.emit(value)
	}
}

func (c *call) emit(value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, ch := range c.chs {
		ch <- value
	}
}

func (c *call) close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, ch := range c.chs {
		close(ch)
	}
	c.chs = nil
}

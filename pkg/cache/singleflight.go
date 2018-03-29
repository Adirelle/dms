package cache

import (
	"sync"
)

type SingleFlight struct {
	calls sync.Map
}

func (f *SingleFlight) Do(key interface{}, fn func(interface{}) (interface{}, bool)) <-chan interface{} {
	ch := make(chan interface{}, 1)
	c := &call{}
	if c2, loaded := f.calls.LoadOrStore(key, c); loaded {
		c = c2.(*call)
	} else {
		go func() {
			c.Run(func() (interface{}, bool) { return fn(key) })
			f.calls.Delete(key)
		}()
	}
	c.Listen(ch)
	return ch
}

type call struct {
	chs []chan<- interface{}
}

func (c *call) Listen(ch chan<- interface{}) {
	c.chs = append(c.chs, ch)
}

func (c *call) Run(fn func() (interface{}, bool)) {
	defer func() {
		for _, ch := range c.chs {
			close(ch)
		}
	}()
	if value, ok := fn(); ok {
		for _, ch := range c.chs {
			ch <- value
		}
	}
}

package cache

import "fmt"

// Printf is a printf-like function to be used with Spy()
type Printf func(string, ...interface{})

type spy struct {
	Cache
	f Printf
}

// Spy logs operations using the given function.
func Spy(f Printf) Option {
	return func(c Cache) Cache {
		return &spy{c, f}
	}
}

func (s *spy) Put(key, value interface{}) (err error) {
	err = s.Cache.Put(key, value)
	s.f("%s.Put(%T(%v), %T(%v)) -> %v", s.Cache, key, key, value, value, err)
	return
}

func (s *spy) Get(key interface{}) (value interface{}, err error) {
	value, err = s.Cache.Get(key)
	s.f("%s.Get(%T(%v)) -> %T(%v), %v", s.Cache, key, key, value, value, err)
	return
}

func (s *spy) Remove(key interface{}) (removed bool) {
	removed = s.Cache.Remove(key)
	s.f("%s.Remove(%T(%v)) -> %v", s.Cache, key, key, removed)
	return
}

func (s *spy) Flush() (err error) {
	err = s.Cache.Flush()
	s.f("%s.Flush() -> %v", s.Cache, err)
	return
}

func (s *spy) Len() (len int) {
	len = s.Cache.Len()
	s.f("%s.Len() -> %v", s.Cache, len)
	return
}

type errorLogger struct {
	Cache
	log Printf
}

// LogErrors catchs and logs errors using the given function.
func LogErrors(f Printf) Option {
	return func(c Cache) Cache {
		return &errorLogger{c, f}
	}
}

func (c *errorLogger) Put(key, value interface{}) (err error) {
	if err := c.Cache.Put(key, value); err != nil {
		c.log("%s.Put(%v, %s): %s", c.Cache, key, value, err)
	}
	return nil
}

func (c *errorLogger) Get(key interface{}) (value interface{}, err error) {
	value, err = c.Cache.Get(key)
	if err != nil && err != ErrKeyNotFound {
		c.log("%s.Get(%v): %s", c.Cache, key, err)
		key = ErrKeyNotFound
	}
	return
}

func (c *errorLogger) Flush() error {
	if err := c.Cache.Flush(); err != nil {
		c.log("%s.Flush(): %s", c.Cache, err)
	}
	return nil
}

// EventType represents the type of operation that has been performed.
type EventType uint8

// EventType values
const (
	UNKNOWN EventType = iota
	PUT
	GET
	REMOVE
	FLUSH
	LEN
)

func (e EventType) String() string {
	switch e {
	case PUT:
		return "PUT"
	case GET:
		return "GET"
	case REMOVE:
		return "REMOVE"
	case FLUSH:
		return "FLUSH"
	case LEN:
		return "LEN"
	default:
		return fmt.Sprintf("EventType(%d)", e)
	}
}

// GoString is fmt.Sprintf("cache.%s", e)
func (e EventType) GoString() string {
	return fmt.Sprintf("cache.%s", e)
}

// Event represents an operation on a cache.
type Event struct {
	// The type of operation
	Type EventType

	// The targetted cache
	Cache Cache

	// The entry key (PUT, GET, REMOVE)
	Key interface{}

	// The entry value (PUT) or any value returned by the operation (GET, REMOVE, LEN).
	Value interface{}

	// Any error returned by the operation (PUT, GET, FLUSH).
	Err error
}

type emitter struct {
	Cache
	ch chan<- Event
}

// Emitter sends cache events to the given channel.
func Emitter(ch chan<- Event) Option {
	return func(c Cache) Cache {
		return &emitter{c, ch}
	}
}

func (e *emitter) emit(t EventType, key, value interface{}, err error) {
	select {
	case e.ch <- Event{t, e.Cache, key, value, err}:
	default:
	}
}

func (e *emitter) Put(key, value interface{}) (err error) {
	err = e.Cache.Put(key, value)
	e.emit(PUT, key, value, err)
	return
}

func (e *emitter) Get(key interface{}) (value interface{}, err error) {
	value, err = e.Cache.Get(key)
	e.emit(GET, key, value, err)
	return
}

func (e *emitter) Remove(key interface{}) (removed bool) {
	removed = e.Cache.Remove(key)
	e.emit(REMOVE, key, removed, nil)
	return
}

func (e *emitter) Flush() (err error) {
	err = e.Cache.Flush()
	e.emit(FLUSH, nil, nil, err)
	return
}

func (e *emitter) Len() (len int) {
	len = e.Cache.Len()
	e.emit(LEN, nil, len, nil)
	return
}

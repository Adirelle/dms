package cache

import "github.com/Adirelle/go-libs/logging"

type Memo interface {
	Get(key interface{}) <-chan interface{}
}

type LoaderFunc func(interface{}) (interface{}, error)

type IsFresher interface {
	IsFresh() bool
}

type memo struct {
	Storage
	f LoaderFunc
	*singleFlight
	logging.Logger
}

func (m *memo) Get(key interface{}) <-chan interface{} {
	return m.Do(key, func() (interface{}, bool) { return m.load(key) })
}

func (m *memo) load(key interface{}) (interface{}, bool) {
	value := m.Fetch(key)
	if value != nil {
		if f, ok := value.(IsFresher); !ok || f.IsFresh() {
			return value, true
		}
	}
	value, err := m.f(key)
	if err != nil {
		m.Warn(err)
		m.Delete(key)
		return nil, false
	}
	m.Store(key, value)
	return value, true
}

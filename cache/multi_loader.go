package cache

import (
	"fmt"
	"log"
	"reflect"
	"time"

	"github.com/bluele/gcache"
)

type MultiLoader map[reflect.Type]gcache.LoaderExpireFunc

func NewMultiLoader() MultiLoader {
	return make(map[reflect.Type]gcache.LoaderExpireFunc)
}

func (m MultiLoader) RegisterLoader(sample_key interface{}, loader gcache.LoaderFunc) {
	m.RegisterLoaderExpire(sample_key, wrapSimpleLoader(loader))
}

func (m MultiLoader) RegisterLoaderExpire(sample_key interface{}, loader gcache.LoaderExpireFunc) {
	t := reflect.TypeOf(sample_key).Elem()
	if _, exists := m[t]; exists {
		log.Panicf("%s already registered", t)
	}
	m[t] = loader
}

func (m MultiLoader) Load(key interface{}) (value interface{}, ttl *time.Duration, err error) {
	t := reflect.TypeOf(key)
	for k, l := range m {
		if t.AssignableTo(k) {
			return l(key)
		}
	}
	return nil, nil, fmt.Errorf("unregistered key type: %s", t)
}

package dic

import (
	"errors"
	"fmt"
	"log"
	"reflect"
	"strings"

	"github.com/Adirelle/go-libs/logging"
)

// ErrInvalidTarget is returned when the target passed to Fetch is not a pointer
var ErrInvalidTarget = errors.New("invalid target to Fetch")

// Container is the generic container interface
type Container interface {
	// Register a new Provider.
	Register(Provider)

	// Fetch sets target to a value matching its type and built from the container.
	Fetch(target interface{}) error
}

// BaseContainer is the container implementation of this package.
type BaseContainer struct {
	providers map[interface{}]Provider
	path      []Provider
	logger    *log.Logger
}

// New initializes new, empty Container, that logs to nothing.
func New() *BaseContainer {
	return &BaseContainer{
		providers: make(map[interface{}]Provider),
		logger:    log.New(nopWriter{}, "", 0),
	}
}

// LogTo sets the container logger, for debugging purpose.
func (c *BaseContainer) LogTo(l *log.Logger) {
	c.logger = l
}

// Register registers the given provider.
//
// It panics if the provider key has already been registered.
func (c *BaseContainer) Register(p Provider) {
	k := p.Key()
	if e, exists := c.providers[k]; exists {
		c.logger.Panicf("%v already registered: %s", k, e)
	}
	c.logger.Printf("Registering %s", p)
	c.providers[k] = p
}

// RegisterFrom uses reflection to register constants and methods from the given struct.
func (c *BaseContainer) RegisterFrom(struc interface{}) {
	v := reflect.ValueOf(struc)

	t := v.Type()
	for i := 0; i < v.NumMethod(); i++ {
		method := v.Method(i)
		name := t.Method(i).Name
		if !isExported(name) {
			continue
		}
		c.Register(Func(method.Interface()))
	}

	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	t = v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		name := t.Field(i).Name
		if !isExported(name) {
			continue
		}
		c.Register(Constant(field.Interface()))
	}
}

/*
Fetch builds a value out of the container to fill the given target, which must be a pointer.

Matching is done by type.

It returns an error in the following cases:
    - the target is not a pointer,
    - there is no provider for the target type,
    - it detects a cycle,
    - the provider returns an error,
    - the provider panics.
*/
func (c *BaseContainer) Fetch(target interface{}) (err error) {
	value := reflect.ValueOf(target)
	if value.Kind() != reflect.Ptr {
		err = ErrInvalidTarget
		return
	}
	value = value.Elem()
	provider, err := c.getProvider(value.Type())
	if err != nil {
		return
	}

	done, err := c.detectCycle(provider)
	if err != nil {
		return
	}
	defer done()

	defer func() {
		if rec := logging.RecoverError(); rec != nil {
			err = &BuildPanicError{provider, rec}
		}
	}()

	ret, err := provider.Provide(c)
	if err == nil {
		if ret.IsValid() {
			value.Set(ret)
		} else {
			err = &BuildError{provider}
		}
	}
	return
}

func (c *BaseContainer) getProvider(key interface{}) (p Provider, err error) {
	p, found := c.providers[key]
	if !found {
		err = &NoProviderError{key}
	}
	return
}

func (c *BaseContainer) detectCycle(p Provider) (f func(), err error) {
	n := len(c.path)
	for i := n - 1; i >= 0; i-- {
		if c.path[i] == p {
			err = &CycleError{c.path[i:]}
			return
		}
	}

	c.path = append(c.path, p)
	f = func() { c.path = c.path[:n] }
	return
}

// func (b *loggingBuilder) Build(p Provider, c Container) (value reflect.Value, err error) {
// 	prev := b.indent
// 	b.L.Debugf("%s├─building %s", b.indent, p)
// 	b.indent += "│  "
// 	value, err = b.Builder.Build(p, c)
// 	if err != nil {
// 		b.L.Warnf("%s└─failed: %s", b.indent, err)
// 	} else {
// 		b.L.Debugf("%s└─success, got %v", b.indent, value.Type())
// 	}
// 	b.indent = prev
// 	return
// }

func isExported(name string) bool {
	first := name[:1]
	return first == strings.ToUpper(first)
}

// NoProviderError is the error returned when there is no provider for a given key in the container.
type NoProviderError struct {
	// The key that was not found.
	Key interface{}
}

func (e *NoProviderError) Error() string {
	return fmt.Sprintf("no provider for %v", e.Key)
}

// BuildPanicError is the error returned when the provider panics.
type BuildPanicError struct {
	// The provider that paniced.
	Provider Provider

	// The panic value as an error.
	Err error
}

func (e *BuildPanicError) Error() string {
	return fmt.Sprintf("%v panic:\n\t%s", e.Provider, e.Err)
}

// BuildError is the error returned when the provider returns an invalid reflect.Value.
type BuildError struct {
	// The provider that failed.
	Provider Provider
}

func (e *BuildError) Error() string {
	return fmt.Sprintf("%v returned an invalid value", e.Provider)
}

// CycleError is the error returned when the container detects a cycle.
type CycleError struct {
	// The list of provider involved in the cycle.
	Providers []Provider
}

func (e *CycleError) Error() string {
	return fmt.Sprintf("cycle involving these providers: %v", e.Providers)
}

type nopWriter struct{}

func (nopWriter) Write(b []byte) (int, error) { return len(b), nil }

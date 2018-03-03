package dic

import (
	"fmt"
	"log"
	"reflect"
	"strings"

	"github.com/anacrolix/dms/logging"
)

type Builder interface {
	fmt.Stringer
	Register(provider Provider, keys ...interface{})
	ProviderFor(key interface{}) (Provider, bool)
	Build(Provider, Container) (reflect.Value, error)
}

type builder map[interface{}]Provider

func (b builder) String() string {
	buf := strings.Builder{}
	for k, v := range b {
		n := ""
		if s, isString := k.(string); isString {
			n = fmt.Sprintf("%q", s)
		} else {
			n = fmt.Sprintf("%s", k)
		}
		buf.WriteString(fmt.Sprintf("  %s: %s,\n", n, v))
	}
	return "{\n" + buf.String() + "}"
}

func (b builder) Register(provider Provider, keys ...interface{}) {
	t := provider.ValueType()
	b[t] = provider
	for _, k := range keys {
		if _, exists := b[k]; exists {
			log.Panicf("duplicate key: %v", k)
		}
		b[k] = provider
	}
}

func (b builder) ProviderFor(k interface{}) (p Provider, ok bool) {
	p, ok = b[k]
	return
}

func (b builder) Build(p Provider, c Container) (value reflect.Value, err error) {
	defer func() {
		if rec := logging.RecoverError(); rec != nil {
			err = &BuildPanicError{p, rec}
		}
	}()
	value, err = p.Provide(c)
	if err == nil && !value.IsValid() {
		err = &BuildError{p}
	}
	return
}

type BuildPanicError struct {
	Provider Provider
	Err      error
}

func (e *BuildPanicError) Error() string {
	return fmt.Sprintf("%v panic:\n\t%s", e.Provider, e.Err)
}

type BuildError struct {
	Provider Provider
}

func (e *BuildError) Error() string {
	return fmt.Sprintf("%v returned an invalid value", e.Provider)
}

type loggingBuilder struct {
	Builder
	L      logging.Logger
	indent string
}

func (b *loggingBuilder) Build(p Provider, c Container) (value reflect.Value, err error) {
	prev := b.indent
	b.L.Debugf("%s├─building %s", b.indent, p)
	b.indent += "│  "
	value, err = b.Builder.Build(p, c)
	if err != nil {
		b.L.Warnf("%s└─failed: %s", b.indent, err)
	} else {
		b.L.Debugf("%s└─success, got %v", b.indent, value.Type())
	}
	b.indent = prev
	return
}

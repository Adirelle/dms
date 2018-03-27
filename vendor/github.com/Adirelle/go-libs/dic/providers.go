package dic

import (
	"fmt"
	"log"
	"reflect"
	"sync"
)

// Provider defines an interface for building values out of a Container.
type Provider interface {
	// Provide is used to build the value.
	// The Container can be used to pull in dependencies needed to build the value.
	Provide(Container) (reflect.Value, error)

	// Key returns a value used to index to this provider in the Container.
	// This can be anything, but the expected types are string and reflect.Type.
	Key() interface{}

	// This is not strictly required but it is very useful for debugging.
	fmt.Stringer
}

// ConstantProvider holds a value to return as is.
type ConstantProvider struct {
	// The provided value
	Value reflect.Value
	Type  reflect.Type
}

// Constant creates a ConstantProvider for the given value.
func Constant(value interface{}) Provider {
	return &ConstantProvider{reflect.ValueOf(value), reflect.TypeOf(value)}
}

func (c *ConstantProvider) String() string {
	return c.Value.Type().String()
}

// Provide simply returns the constant.
func (c *ConstantProvider) Provide(Container) (reflect.Value, error) {
	return c.Value, nil
}

// Key returns the constant type.
func (c *ConstantProvider) Key() interface{} {
	return c.Type
}

// FuncProvider wraps a function to build the wanted value from arguments pulled from the container.
type FuncProvider struct {
	// The function itself.
	Func reflect.Value

	// The types of its arguments.
	ArgumentTypes []reflect.Type

	// The type of the firstr returned valued.
	ReturnType reflect.Type

	// Indicates that the function returns an error in second position.
	ReturnsError bool
}

/*
Func builds a FuncProvider for the given function.

The returned provided is a Singleton, to ensure the function is called only once.

Func panics if the function does not respect the following conditions:

    * The function returns less than one value or more than two.
    * If the function returns two values, the second one must be of type error.

*/
func Func(fn interface{}) Provider {
	t := validateProviderFunc(fn)
	f := &FuncProvider{
		Func:          reflect.ValueOf(fn),
		ArgumentTypes: make([]reflect.Type, t.NumIn()),
		ReturnType:    t.Out(0),
		ReturnsError:  t.NumOut() == 2,
	}
	for i := 0; i < t.NumIn(); i++ {
		f.ArgumentTypes[i] = t.In(i)
	}
	return &Singleton{Provider: f}
}

func validateProviderFunc(fn interface{}) (t reflect.Type) {
	t = reflect.TypeOf(fn)
	if t.Kind() != reflect.Func {
		log.Panicf("Func argument must be a Func: %#v is a %s", fn, t.Kind())
	} else if t.NumOut() < 1 {
		log.Panicf("Func argument must return at least one value: %#v does not", fn)
	} else if t.NumOut() > 2 {
		log.Panicf("Func argument must return at most two values: %#v does not", fn)
	} else if t.NumOut() == 2 && t.Out(1).String() != "error" {
		log.Panicf("The second argument of Func argument must be of type 'error': %s is not", t.Out(1))
	}
	return
}

// String returns the function signature.
func (p *FuncProvider) String() string {
	return p.Func.Type().String()
}

/*
Provide fetchs the function argments by type from the container and then call the functions.

If the function returns an error, it is wrapped and returned by Provide.
*/
func (p *FuncProvider) Provide(container Container) (value reflect.Value, err error) {
	args := make([]reflect.Value, len(p.ArgumentTypes))
	for i, t := range p.ArgumentTypes {
		ptr := reflect.New(t)
		err = container.Fetch(ptr.Interface())
		if err != nil {
			err = &FuncArgumentError{p, err, i}
			return
		}
		args[i] = ptr.Elem()
	}
	results := p.Func.Call(args)
	value = results[0]
	if p.ReturnsError && !results[1].IsNil() {
		err = &FuncCallError{p, results[1].Interface().(error), args}
	}
	return
}

// Key returns the type of the first return value of the function.
func (p *FuncProvider) Key() interface{} {
	return p.ReturnType
}

// FuncCallError is returned when the func returned an actual error as its second return value.
type FuncCallError struct {
	// The provider that failed.
	Func *FuncProvider

	// The returned error.
	Err error

	// The arguments that was passed to the function.
	Args []reflect.Value
}

func (e *FuncCallError) Error() string {
	return fmt.Sprintf("call to %s with %v returned:\n\t%s", e.Func, e.Args, e.Err)
}

// FuncArgumentError is returned by FuncProvider.Provider when an argument cannot be pulled from the container.
type FuncArgumentError struct {
	// The provider that failed.
	Func *FuncProvider

	// The returned error.
	Err error

	// The argument position.
	Index int
}

func (e *FuncArgumentError) Error() string {
	return fmt.Sprintf("cannot inject argument #%d of %s:\n\t%s", e.Index, e.Func, e.Err)
}

// Singleton wraps another provider to guarantee it is used only once.
type Singleton struct {
	// The actual provider
	Provider
	once  sync.Once
	value reflect.Value
	err   error
}

func (s *Singleton) String() string {
	return fmt.Sprintf("Singleton(%s)", s.Provider)
}

// Provide executes the actual providers and returns the values.
// Subsequent calls to Provide always return the same values.
func (s *Singleton) Provide(c Container) (reflect.Value, error) {
	s.once.Do(func() {
		s.value, s.err = s.Provider.Provide(c)
	})
	return s.value, s.err
}

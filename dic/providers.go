package dic

import (
	"fmt"
	"log"
	"reflect"
	"strings"
)

type Provider interface {
	fmt.Stringer
	Provide(Container) (reflect.Value, error)
	ValueType() reflect.Type
}

func Auto(v interface{}) Provider {
	if p, isProvider := v.(Provider); isProvider {
		return p
	}
	switch reflect.ValueOf(v).Type().Kind() {
	case reflect.Func:
		return Func(v)
	case reflect.Struct:
		return Struct(v)
	}
	return Constant(v)
}

type constant reflect.Value

func Constant(value interface{}) Provider {
	return constant(reflect.ValueOf(value))
}

func (c constant) String() string {
	return reflect.Value(c).String()
}

func (c constant) Provide(Container) (reflect.Value, error) {
	return reflect.Value(c), nil
}

func (c constant) ValueType() reflect.Type {
	return reflect.Value(c).Type()
}

type structProvider struct{ structType reflect.Type }

func Struct(zero interface{}) Provider {
	t := reflect.ValueOf(zero).Type()
	if t.Kind() != reflect.Struct {
		log.Panicf("Struct argument must be a struct, not a %s", t.Kind())
	}
	return structProvider{t}
}

func (p structProvider) String() string {
	return p.structType.String()
}

func (p structProvider) Provide(container Container) (value reflect.Value, err error) {
	typ := p.structType
	value = reflect.New(typ).Elem()
	num := typ.NumField()
	for i := 0; i < num; i++ {
		field := typ.Field(i)
		if strings.ToLower(field.Name[:1]) == field.Name[:1] {
			continue
		}
		var val reflect.Value
		val, err = container.get(field.Name, field.Type)
		if err != nil {
			err = &FieldError{field, err}
			return
		}
		value.Field(i).Set(val)
	}
	return
}

func (p structProvider) ValueType() reflect.Type {
	return p.structType
}

type FieldError struct {
	Field reflect.StructField
	Err   error
}

func (e *FieldError) Error() string {
	return fmt.Sprintf("cannot inject %s(%s): %s", e.Field.Name, e.Field.Type, e.Err)
}

type funcProvider struct{ fn reflect.Value }

func Func(fn interface{}) Provider {
	p := &funcProvider{fn: reflect.ValueOf(fn)}
	funcType := p.fn.Type()
	if funcType.Kind() != reflect.Func {
		log.Panicf("ProviderFunc argument must be a func, not a %s", funcType.Kind())
	}
	if funcType.NumOut() < 1 {
		log.Panicf("ProviderFunc argument must return at least one value, %#v does not", fn)
	}
	if funcType.NumOut() > 2 {
		log.Panicf("ProviderFunc argument must return at most two values, %#v does not", fn)
	}
	if funcType.NumOut() == 2 && funcType.Out(1).String() != "error" {
		log.Panicf("ProviderFunc second argument must be of type 'error', not %q", funcType.Out(1))
	}
	return p
}

func (p *funcProvider) String() string {
	return p.fn.String()
}

func (p *funcProvider) Provide(container Container) (value reflect.Value, err error) {
	typ := p.fn.Type()
	numArgs := typ.NumIn()
	args := make([]reflect.Value, 0, numArgs)
	for i := 0; i < numArgs; i++ {
		var arg reflect.Value
		arg, err = container.get(typ.In(i))
		if err != nil {
			err = &FuncArgumentError{p.fn, err, i}
			return
		}
		args = append(args, arg)
	}
	results := p.fn.Call(args)
	value = results[0]
	if len(results) == 2 {
		var isErr bool
		err, isErr = results[1].Interface().(error)
		if isErr && err != nil {
			err = &FuncCallError{p.fn, err, args}
		}
	}
	return
}

func (p *funcProvider) ValueType() reflect.Type {
	return p.fn.Type().Out(0)
}

type FuncCallError struct {
	Func reflect.Value
	Err  error
	Args []reflect.Value
}

func (e *FuncCallError) Error() string {
	return fmt.Sprintf("%v(%v) returned:\n\t%s", e.Func.Type(), e.Args, e.Err)
}

type FuncArgumentError struct {
	Func  reflect.Value
	Err   error
	Index int
}

func (e *FuncArgumentError) Error() string {
	return fmt.Sprintf("cannot inject argument #%d of %v:\n\t%s", e.Index, e.Func.Type(), e.Err)
}

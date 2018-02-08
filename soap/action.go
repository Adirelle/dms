package soap

import (
	"encoding/xml"
	"errors"
	"log"
	"net/http"
	"reflect"
)

// Action is a SOAP action
type Action interface {
	Name() xml.Name
	EmptyArguments() interface{}
	Handle(interface{}, *http.Request) (interface{}, error)
}

// ActionFunc converts a function into an Action.
// The function must conform to the func(A, *http.Request) (B, error) signature
// where A and B are struct types.
func ActionFunc(name xml.Name, f interface{}) Action {
	v := reflect.ValueOf(f)
	t := v.Type()
	err := validateActionFuncType(t)
	if err != nil {
		log.Panicf("soap.ActionFunc(%q, %s): %s", name, t.String(), err.Error())
	}
	return &actionFunc{
		name:      name,
		argType:   t.In(0),
		funcValue: v,
	}
}

var (
	ErrNotAFunc                   = errors.New("must be a func")
	ErrWrongArgumentCount         = errors.New("must have exactly two arguments")
	ErrFirstArgumentWrongType     = errors.New("first argument must be a struct")
	ErrSecondArgumentWrongType    = errors.New("second argument must be *http.Request")
	ErrWrongReturnValueCount      = errors.New("must have exactly two return values")
	ErrFirstReturnValueWrongType  = errors.New("first return value must be a struct")
	ErrSecondReturnValueWrongType = errors.New("second return value must be error")
)

func validateActionFuncType(t reflect.Type) error {
	if t.Kind() != reflect.Func {
		return ErrNotAFunc
	}
	if t.NumIn() != 2 {
		return ErrWrongArgumentCount
	}
	if t.In(0).Kind() != reflect.Struct {
		return ErrFirstArgumentWrongType
	}
	if t.In(1).String() != "*http.Request" {
		return ErrSecondArgumentWrongType
	}
	if t.NumOut() != 2 {
		return ErrWrongReturnValueCount
	}
	if t.Out(0).Kind() != reflect.Struct {
		return ErrFirstReturnValueWrongType
	}
	if t.Out(1).String() != "error" {
		return ErrSecondReturnValueWrongType
	}
	return nil
}

type actionFunc struct {
	name      xml.Name
	argType   reflect.Type
	funcValue reflect.Value
}

func (af *actionFunc) Name() xml.Name {
	return af.name
}

func (af *actionFunc) EmptyArguments() interface{} {
	return reflect.Zero(af.argType).Interface()
}

func (af *actionFunc) Handle(arg interface{}, r *http.Request) (res interface{}, err error) {
	rets := af.funcValue.Call([]reflect.Value{reflect.ValueOf(arg), reflect.ValueOf(r)})
	res = rets[0].Interface()
	if !rets[1].IsNil() {
		err = rets[1].Interface().(error)
	}
	return
}

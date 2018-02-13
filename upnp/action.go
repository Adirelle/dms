package upnp

import (
	"reflect"

	"github.com/anacrolix/dms/soap"
)

type Action interface {
	soap.Action
	EmptyReturnValue() interface{}
}

type actionFunc struct {
	soap.Action
	returnType reflect.Type
}

func (a *actionFunc) EmptyReturnValue() interface{} {
	return reflect.Zero(a.returnType).Interface()
}

func ActionFunc(f interface{}) Action {
	return &actionFunc{
		Action:     soap.ActionFunc(f),
		returnType: reflect.TypeOf(f).Out(0),
	}
}

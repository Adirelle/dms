package upnp

import (
	"reflect"

	"github.com/Adirelle/dms/pkg/soap"
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

func ActionFunc(f interface{}) (a Action, err error) {
	soapAction, err := soap.ActionFunc(f)
	if err == nil {
		a = &actionFunc{
			Action:     soapAction,
			returnType: reflect.TypeOf(f).Out(0),
		}
	}
	return
}

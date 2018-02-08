package soap

import (
	"encoding/xml"
	"fmt"
	"reflect"
)

// Fault is used to send errors
type Fault struct {
	XMLName xml.Name `xml:"Fault"`
	Code    string   `xml:"faultcode"`
	Message string   `xml:"faultstring"`
	Actor   string   `xml:"faultactor,omitempty"`
	Detail  string   `xml:"detail,omitempty"`
}

func (f *Fault) Error() string {
	return f.Message
}

// Error converts any error into a SOAP Fault
func ConvertError(code string, err error) *Fault {
	if fault, ok := err.(*Fault); ok {
		return fault
	}
	return &Fault{
		Code:    code,
		Message: reflect.TypeOf(err).String(),
		Detail:  err.Error(),
	}
}

// Errorf creates a SOAP Fault from an error message
func Errorf(code, msg string, args ...interface{}) *Fault {
	return &Fault{Code: code, Message: fmt.Sprintf(msg, args...)}
}

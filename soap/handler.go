package soap

import (
	"encoding/xml"
	"errors"
	"fmt"
	"log"
	"net/http"
	"reflect"

	"github.com/anacrolix/dms/logging"
)

// Action is a SOAP action
type Action interface {
	Name() xml.Name
	EmptyArguments() interface{}
	Handle(interface{}, *http.Request) (interface{}, error)
}

// Server holds the action map and can serve SOAP through HTTP
type Server struct {
	actions map[xml.Name]Action
	l       logging.Logger
}

// New creates an empty SOAP Server
func New(l logging.Logger) *Server {
	return &Server{make(map[xml.Name]Action), l.Named("soap")}
}

// RegisterAction adds a Handler for a given action
func (s *Server) RegisterAction(action Action) {
	name := action.Name()
	if _, exist := s.actions[name]; exist {
		s.l.DPanicf("action already registered: %s", name)
		return
	}
	s.actions[name] = action
}

var (
	responseHeader = []byte(`<?xml version="1.0" encoding="UTF-8"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body>`)
	responseFooter = []byte(`</s:Body></s:Envelope>`)
)

// ServeHTTP implements http.Handler
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log := logging.FromRequest(r)
	res, err := s.serve(r)
	if err != nil {
		res = ConvertError("s:Server", err)
		log.Warn(err.Error())
	}

	w.Header().Set("Content-Type", "application/soap+xml; charset=UTF-8")
	if _, err = w.Write(responseHeader); err == nil {
		if err = xml.NewEncoder(w).Encode(res); err == nil {
			_, err = w.Write(responseFooter)
		}
	}
	if err != nil {
		log.Warnf("error marshalling SOAP response: %s", err.Error())
	}
}

func (s *Server) serve(r *http.Request) (res interface{}, err error) {
	env := envelope{}
	payload := &(env.Body.Payload)
	payload.actions = s.actions
	if err = xml.NewDecoder(r.Body).Decode(&env); err == nil {
		res, err = payload.action.Handle(payload.value, r)
	} else {
		err = ConvertError("s:Client", err)
	}
	return
}

type envelope struct {
	XMLName       xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Envelope"`
	EncodingStyle string   `xml:"encodingStyle,attr"`
	Body          struct {
		Payload payload `xml:",any"`
	} `xml:"Body"`
}

type payload struct {
	actions map[xml.Name]Action
	action  Action
	value   interface{}
}

// UnmarshalXML creates a new value of type unmarshalType and unmarshals the XML element into it.
func (p *payload) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	var known bool
	p.action, known = p.actions[start.Name]
	if !known {
		return Errorf("s:Client", "unknown action %s", start.Name)
	}
	ptr := reflect.New(reflect.TypeOf(p.action.EmptyArguments()))
	err := d.DecodeElement(ptr.Interface(), &start)
	p.value = reflect.Indirect(ptr).Interface()
	return err
}

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
		Message: err.Error(),
		Detail:  fmt.Sprintf("%#v", err),
	}
}

// Errorf creates a SOAP Fault from an error message
func Errorf(code, msg string, args ...interface{}) *Fault {
	return &Fault{Code: code, Message: fmt.Sprintf(msg, args...)}
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

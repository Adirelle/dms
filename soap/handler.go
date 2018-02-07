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
	Name() string
	EmptyArguments() interface{}
	Handle(interface{}, *http.Request) (interface{}, error)
}

// Server holds the action map and can serve SOAP through HTTP
type Server struct {
	actions map[string]Action
	l       logging.Logger
}

// New creates an empty SOAP Server
func New(l logging.Logger) *Server {
	return &Server{make(map[string]Action), l.Named("soap")}
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

// ServeHTTP implements http.Handler
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log := logging.FromRequest(r)

	name, err := s.parseActionName(r)
	if err != nil {
		log.Warn(err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	action, found := s.actions[name]
	if !found {
		err = fmt.Errorf("Unhandled SOAP action: %q", name)
		log.Warn(err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	log = log.With("action", action.Name())

	env := envelope{}
	env.Body.unmarshalType = reflect.TypeOf(action.EmptyArguments())
	err = xml.NewDecoder(r.Body).Decode(&env)
	if err != nil {
		msg := fmt.Sprintf("Could not parse request: %s", err.Error())
		log.Warn(msg)
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	res, err := action.Handle(env.Body.value, r)
	if err != nil {
		log.Warnf("error while processing %q action: %s", name, err.Error())
		res = Error(err)
	}
	env.Body.value = res

	w.Header().Set("Content-Type", "application/soap+xml; charset=UTF-8")
	err = xml.NewEncoder(w).Encode(env)
	if err != nil {
		log.Warnf("error marshalling SOAP response: %s", err.Error())
	}
}

func (s *Server) parseActionName(r *http.Request) (name string, err error) {
	action := r.Header.Get("SoapAction")
	if action == "" {
		err = fmt.Errorf("missing SoapAction header")
	} else if action[0] != '"' || action[len(action)-1] != '"' {
		err = fmt.Errorf("invalid SoapAction header: %q", action)
	} else {
		name = action[1 : len(action)-1]
	}
	return
}

type envelope struct {
	XMLName       xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Envelope"`
	EncodingStyle string   `xml:"encodingStyle,attr"`
	Body          payload  `xml:"Body"`
}

type payload struct {
	value         interface{}
	unmarshalType reflect.Type
}

// UnmarshalXML creates a new value of type unmarshalType and unmarshals the XML element into it.
func (p *payload) UnmarshalXML(d *xml.Decoder, start xml.StartElement) error {
	ptr := reflect.New(p.unmarshalType)
	err := d.Decode(ptr.Interface())
	if err == nil {
		p.value = reflect.Indirect(ptr).Interface()
		d.Skip()
	}
	return err
}

// MarshalXML marshals the Value as is.
func (p payload) MarshalXML(e *xml.Encoder, start xml.StartElement) (err error) {
	if err = e.EncodeToken(start); err != nil {
		return
	}
	if err = e.Encode(p.value); err != nil {
		return
	}
	return e.EncodeToken(start.End())
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
func Error(err error) *Fault {
	if fault, ok := err.(*Fault); ok {
		return fault
	}
	return &Fault{
		Code:    "Client",
		Message: err.Error(),
		Detail:  fmt.Sprintf("%#v", err),
	}
}

// Errorf creates a SOAP Fault from an error message
func Errorf(msg string, args ...interface{}) *Fault {
	return &Fault{
		Code:    "Client",
		Message: fmt.Sprintf(msg, args...),
		Detail:  fmt.Sprintf("%#v", args),
	}
}

// ActionFunc converts a function into an Action.
// The function must conform to the func(A, *http.Request) (B, error) signature
// where A and B are struct types.
func ActionFunc(name string, f interface{}) Action {
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
	name      string
	argType   reflect.Type
	funcValue reflect.Value
}

func (af *actionFunc) Name() string {
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

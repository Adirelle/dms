package soap

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"

	"github.com/anacrolix/dms/logging"
)

// Call is a call to an action
type Call interface {
	Action() string
	DecodeArguments(interface{}) error
	EncodeResponse(interface{}) error
	io.Writer
}

// Handler handles calls
type Handler interface {
	Handle(Call)
}

// HandlerFunc is a function that can be used as a Handler
type HandlerFunc func(Call)

// Handle implements the Handler interface
func (f HandlerFunc) Handle(r Call) {
	f(r)
}

// Server holds the action map and can serve SOAP through HTTP
type Server struct {
	actions map[string]Handler
	l       logging.Logger
}

// New creates an empty SOAP Server
func New(l logging.Logger) *Server {
	return &Server{make(map[string]Handler), l.Named("soap")}
}

// RegisterAction adds a Handler for a given action
func (s *Server) RegisterAction(action string, h Handler) {
	if _, exist := s.actions[action]; exist {
		s.l.DPanicf("action already registered: %s", action)
		return
	}
	s.actions[action] = h
}

// ServeHTTP implements http.Handler
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c := &call{ResponseWriter: w}
	if err := c.readFromRequest(r); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if handler, found := s.actions[c.action]; found {
		handler.Handle(c)
	} else {
		c.EncodeResponse(fmt.Errorf("unknown SOAP action %q", c.action))
	}
}

// Envelope is the outermost SOAP element
type Envelope struct {
	XMLName       xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Envelope"`
	EncodingStyle string   `xml:"encodingStyle,attr"`
	Body          Body
}

// Body contains the payload in SOAP messages
type Body struct {
	XMLName xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Body"`
	Content []byte   `xml:",innerxml"`
}

// Fault is used to send errors
type Fault struct {
	XMLName xml.Name `xml:"http://schemas.xmlsoap.org/soap/envelope/ Fault"`
	Code    string   `xml:"faultcode"`
	Message string   `xml:"faultstring"`
	Actor   string   `xml:"faultactor,omitempty"`
	Detail  string   `xml:"detail,omitempty"`
}

func (f *Fault) Error() string {
	return f.Message
}

type call struct {
	http.ResponseWriter
	action string
	body   []byte
}

func (c *call) readFromRequest(r *http.Request) error {
	defer r.Body.Close()
	c.action = r.Header.Get("SOAPACTION")
	if c.action == "" {
		return fmt.Errorf("Missing SoapAction header")
	}
	var env Envelope
	if err := xml.NewDecoder(r.Body).Decode(&env); err != nil {
		return err
	}
	c.body = env.Body.Content
	return nil
}

func (c *call) Action() string {
	return c.action
}

func (c *call) DecodeArguments(target interface{}) error {
	return xml.Unmarshal(c.body, target)
}

const envelopeHeader = `<?xml version="1.0" encoding="UTF-8"?>` +
	`<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">` +
	`<soapenv:Body>`

const envelopeFooter = `</soapenv:Body></soapenv:Envelope>`

func (c *call) EncodeResponse(value interface{}) (err error) {
	c.Header().Set("Content-Type", "application/soap+xml; charset=utf-8")
	if valueErr, ok := value.(error); ok {
		value = Error(valueErr)
	}

	_, err = c.Write([]byte(envelopeHeader))
	if err != nil {
		return
	}

	err = xml.NewEncoder(c).Encode(value)
	if err != nil {
		return
	}

	_, err = c.Write([]byte(envelopeFooter))

	return
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

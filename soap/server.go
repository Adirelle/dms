package soap

import (
	"encoding/xml"
	"net/http"
	"reflect"
	"strconv"

	"github.com/anacrolix/dms/logging"
	"go.uber.org/zap/buffer"
)

var bufferPool = buffer.NewPool()

// Server holds the action map and can serve SOAP through HTTP
type Server struct {
	actions map[xml.Name]Action
	l       logging.Logger
}

// New creates an empty SOAP Server
func New(l logging.Logger) *Server {
	return &Server{make(map[xml.Name]Action), l}
}

// RegisterAction adds a Handler for a given action
func (s *Server) RegisterAction(name xml.Name, action Action) {
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

	b := bufferPool.Get()
	defer b.Free()
	if _, err = b.Write(responseHeader); err == nil {
		if err = xml.NewEncoder(b).Encode(res); err == nil {
			_, err = b.Write(responseFooter)
		}
	}

	l := b.Len()
	w.Header().Set("Content-Length", strconv.Itoa(l))
	w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)

	n, err := w.Write(b.Bytes())
	if err != nil {
		log.Warnf("error marshalling SOAP response: %s", err.Error())
	} else if n < l {
		log.Warnf("short write: %s/%s", n, l)
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

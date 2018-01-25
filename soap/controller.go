package soap

import (
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
)

// Request holds the SOAP request
type Request struct {
	// Content of the SoapAction header
	Action string
	// XML enveloppe Body
	Body []byte
	// The HTTP request carrying the SOAP request
	HTTP *http.Request
}

// Service is a SOAP Server
type Service interface {
	Supports(string) bool
	ServeSOAP(Request) (interface{}, error)
}

// Controller is an http.Handler that provides several SOAP services
type Controller struct {
	services []Service
}

// RegisterService adds a SOAP service to the controller
func (c *Controller) RegisterService(s Service) {
	c.services = append(c.services, s)
}

// Process a SOAP request
func (c *Controller) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	action := r.Header.Get("SoapAction")
	if action == "" {
		http.Error(w, "Missing SaopAction header", http.StatusBadRequest)
		return
	}
	code := http.StatusOK
	response, err := c.serve(r, action)
	if err != nil {
		// TODO: better error handling
		response = NewFault(err.Error(), fmt.Sprintf("%+v", err))
		code = http.StatusBadRequest
	}
	c.sendSoapResponse(w, code, response)
}

func (c *Controller) serve(r *http.Request, action string) (response interface{}, err error) {
	service, found := c.findServiceFor(action)
	if !found {
		err = fmt.Errorf("Unknown action %q", action)
		return
	}
	env, err := c.parseSoapEnveloppe(r)
	if err != nil {
		return
	}
	response, err = service.ServeSOAP(Request{action, env.Body.Action, r})
	return
}

func (c *Controller) findServiceFor(action string) (Service, bool) {
	for _, service := range c.services {
		if service.Supports(action) {
			return service, true
		}
	}
	return nil, false
}

func (c *Controller) parseSoapEnveloppe(r *http.Request) (env Envelope, err error) {
	err = xml.NewDecoder(r.Body).Decode(&env)
	return
}

func (c *Controller) sendSoapResponse(w http.ResponseWriter, code int, response interface{}) {
	w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
	w.Header().Set("Ext", "")
	w.WriteHeader(code)
	w.Write([]byte(`<?xml version="1.0" encoding="utf-8" standalone="yes"?><s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/"><s:Body>`))
	err := xml.NewEncoder(w).Encode(response)
	w.Write([]byte(`</s:Body></s:Envelope>`))
	if err != nil {
		log.Printf("Error xml-encoding response: %s", err.Error())
	}
}

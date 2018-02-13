package upnp

import (
	"bytes"
	"encoding/xml"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/anacrolix/dms/logging"
	"github.com/anacrolix/dms/soap"
	"github.com/gorilla/mux"
)

const (
	DDDRoute     = "ddd"
	ControlRoute = "control"
	SCPDRoute    = "scpd"
	IconRoute    = "icon"
)

type Device interface {
	AddIcon(width, height, depth int, mimeType string, content []byte)
	AddService(*Service)

	DDDLocation() *url.URL
	DeviceTypes() []string
	ServiceTypes() []string
}

type rootDevice struct {
	XMLName     xml.Name    `xml:"urn:schemas-upnp-org:device-1-0 root"`
	SpecVersion specVersion `xml:"specVersion"`
	URLBase     string
	Device      *device `xml:"device"`
}

type specVersion struct {
	Major int `xml:"major"`
	Minor int `xml:"minor"`
}

type DeviceSpec struct {
	DeviceType       string `xml:"deviceType"`
	FriendlyName     string `xml:"friendlyName"`
	Manufacturer     string `xml:"manufacturer"`
	ManufacturerURL  string `xml:"manufacturerURL"`
	ModelDescription string `xml:"modelDescription"`
	ModelName        string `xml:"modelName"`
	ModelNumber      uint   `xml:"modelNumber"`
	ModelURL         string `xml:"modelURL"`
	UDN              string
	UPC              string
}

type device struct {
	DeviceSpec
	Icons    []*iconDesc    `xml:"iconList>icon"`
	Services []*serviceDesc `xml:"serviceList>service"`

	router *mux.Router
	soap   *soap.Server
	logger logging.Logger
	sync.Mutex
}

type iconDesc struct {
	Mimetype string `xml:"mimetype"`
	URL      string `xml:"url"`
	Width    int    `xml:"width"`
	Height   int    `xml:"height"`
	Depth    int    `xml:"depth"`

	content []byte
}

type serviceDesc struct {
	URN         string `xml:"serviceType"`
	ID          string `xml:"serviceId"`
	ControlURL  string `xml:"controlURL"`
	EventSubURL string `xml:"eventSubURL"`
	SCPDURL     string `xml:"SCPDURL"`

	service *Service
}

func must(err error) {
	if err != nil {
		log.Panic(err)
	}
}

func NewDevice(spec DeviceSpec, router *mux.Router, logger logging.Logger) Device {

	dev := &device{
		DeviceSpec: spec,
		router:     router,
		logger:     logger,
		soap:       soap.New(logger),
	}

	must(router.Methods("GET").
		Path("/device.xml").
		Name(DDDRoute).
		HandlerFunc(dev.describeDevice).
		GetError())

	must(router.Methods("POST").
		Path("/control").
		HeadersRegexp("Content-Type", "(application|text)/(soap+)?xml").
		Name(ControlRoute).
		Handler(dev.soap).
		GetError())

	must(router.Methods("GET").
		Path("/scpd/{service:[0-9]+}.xml").
		Name(SCPDRoute).
		HandlerFunc(dev.describeService).
		GetError())

	must(router.Methods("GET").
		Path("/icons/{icon:[0-9]+}").
		Name(IconRoute).
		HandlerFunc(dev.serveIcon).
		GetError())

	return dev
}

func (d *device) AddIcon(width, height, depth int, mimeType string, content []byte) {
	idx := len(d.Icons)
	desc := &iconDesc{
		Mimetype: mimeType,
		Width:    width,
		Height:   height,
		Depth:    depth,
		content:  content,
	}
	d.Icons = append(d.Icons, desc)

	if url, err := d.router.Get(IconRoute).URLPath("icon", strconv.Itoa(idx)); err == nil {
		desc.URL = url.String()
	} else {
		d.logger.Warnf("cannot build icon URL: %s", err)
	}
}

func (d *device) AddService(s *Service) {
	idx := len(d.Services)
	desc := &serviceDesc{ID: s.id, URN: s.urn, service: s}
	d.Services = append(d.Services, desc)

	if url, err := d.router.Get(ControlRoute).URLPath(); err == nil {
		desc.ControlURL = url.String()
	} else {
		d.logger.Errorf("cannot build control URL: %s", err)
	}

	if url, err := d.router.Get(SCPDRoute).URLPath("service", strconv.Itoa(idx)); err == nil {
		desc.SCPDURL = url.String()
	} else {
		d.logger.Errorf("cannot build SCPD URL: %s", err)
	}

	desc.EventSubURL = "/sub"

	for name, action := range s.actions {
		d.soap.RegisterAction(xml.Name{s.urn, name}, action)
	}
}

func (d *device) DDDLocation() (res *url.URL) {
	res, err := d.router.Get(DDDRoute).URL()
	if err != nil {
		d.logger.DPanic("Could not build DDD URL: %s", err)
	}
	return
}

func (d *device) DeviceTypes() []string {
	return []string{d.DeviceType}
}

func (d *device) ServiceTypes() (res []string) {
	res = make([]string, len(d.Services))
	for i, s := range d.Services {
		res[i] = s.URN
	}
	return
}

func (d *device) describeDevice(w http.ResponseWriter, r *http.Request) {
	urlBase := &url.URL{Scheme: "http", Host: r.Host, Path: ""}

	w.Header().Set("Content-Type", `text/xml; encoding="UTF-8"`)
	_, err := w.Write([]byte(xml.Header))
	if err == nil {
		err = xml.NewEncoder(w).Encode(rootDevice{
			SpecVersion: specVersion{1, 0},
			URLBase:     urlBase.String(),
			Device:      d,
		})
	}
	if err != nil {
		d.logger.Warnf(err.Error())
	}
}

func (d *device) describeService(w http.ResponseWriter, r *http.Request) {
	idx, err := strconv.Atoi(mux.Vars(r)["service"])
	if err != nil {
		http.Error(w, "Unknown service", http.StatusNotFound)
	}
	service := d.Services[idx].service

	w.Header().Set("Content-Type", `text/xml; encoding="UTF-8"`)
	_, err = w.Write([]byte(xml.Header))
	if err == nil {
		err = xml.NewEncoder(w).Encode(service)
	}
	if err != nil {
		d.logger.Warnf(err.Error())
	}
}

func (d *device) serveIcon(w http.ResponseWriter, r *http.Request) {
	iconVar := mux.Vars(r)["icon"]
	idx, err := strconv.Atoi(iconVar)
	if err != nil {
		http.Error(w, "Unknown icon", http.StatusNotFound)
	}
	icon := d.Icons[idx]

	w.Header().Set("Content-Type", icon.Mimetype)
	http.ServeContent(w, r, iconVar, time.Now(), bytes.NewReader(icon.content))
}

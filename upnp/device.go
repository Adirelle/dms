package upnp

import (
	"bytes"
	"encoding/xml"
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
	AddIcon(int, int, int, string, []byte)
	AddService(*Service)
}

type rootDevice struct {
	XMLName     xml.Name    `xml:"urn:schemas-upnp-org:device-1-0 root"`
	SpecVersion specVersion `xml:"specVersion"`
	Device      *device     `xml:"device"`
}

type specVersion struct {
	Major int `xml:"major"`
	Minor int `xml:"minor"`
}

type device struct {
	DeviceType   string         `xml:"deviceType"`
	FriendlyName string         `xml:"friendlyName"`
	Manufacturer string         `xml:"manufacturer"`
	ModelName    string         `xml:"modelName"`
	UDN          string         `xml:"UDN"`
	Services     []*serviceDesc `xml:"serviceList>service"`
	Icons        []*iconDesc    `xml:"iconList>icon"`

	router *mux.Router
	soap   *soap.Server
	logger logging.Logger
	sync.Mutex
}

type iconDesc struct {
	Mimetype string   `xml:"mimetype"`
	Width    int      `xml:"width"`
	Height   int      `xml:"height"`
	Depth    int      `xml:"depth"`
	URL      *url.URL `xml:"url"`

	content []byte
}

type serviceDesc struct {
	ID          string   `xml:"serviceId"`
	URN         string   `xml:"serviceType"`
	SCPDURL     *url.URL `xml:"SCPDURL"`
	ControlURL  *url.URL `xml:"controlURL"`
	EventSubURL *url.URL `xml:"eventSubURL"`

	service *Service
}

func NewDevice(devType, name, manufacturer, modelName, udn string, router *mux.Router,
	logger logging.Logger) Device {

	dev := &device{
		DeviceType:   devType,
		FriendlyName: name,
		Manufacturer: manufacturer,
		ModelName:    modelName,
		UDN:          udn,
		router:       router,
		logger:       logger,
		soap:         soap.New(logger),
	}

	router.Host("{host}").
		Methods("GET").
		Path("device.xml").
		Name(DDDRoute).
		HandlerFunc(dev.describeDevice)

	router.Methods("POST").
		Path("control").
		HeadersRegexp("Content-Type", "(application|text)/(soap+)?xml").
		Name(ControlRoute).
		Handler(dev.soap)

	router.Methods("GET").
		Path("scpd/{service:[0-9]+}.xml").
		Name(SCPDRoute).
		HandlerFunc(dev.describeService)

	router.Methods("GET").
		Path("icons/{icon:[0-9]+}").
		Name(IconRoute).
		HandlerFunc(dev.serveIcon)

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

	var err error
	if desc.URL, err = d.router.Get(IconRoute).URLPath("icon", strconv.Itoa(idx)); err != nil {
		d.logger.Errorf("cannot build icon URL: %s", err)
	}
}

func (d *device) AddService(s *Service) {
	idx := len(d.Services)
	desc := &serviceDesc{ID: s.id, URN: s.urn, service: s}
	d.Services = append(d.Services, desc)

	var err error
	if desc.ControlURL, err = d.router.Get(ControlRoute).URLPath(); err != nil {
		d.logger.Errorf("cannot build control URL: %s", err)
	}
	if desc.SCPDURL, err = d.router.Get(SCPDRoute).URLPath("service", strconv.Itoa(idx)); err != nil {
		d.logger.Errorf("cannot build SCPD URL: %s", err)
	}

	for name, action := range s.actions {
		d.soap.RegisterAction(xml.Name{s.urn, name}, action)
	}
}

func (d *device) describeDevice(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", `text/xml; encoding="UTF-8"`)
	_, err := w.Write([]byte(xml.Header))
	if err == nil {
		err = xml.NewEncoder(w).Encode(rootDevice{SpecVersion: specVersion{1, 0}, Device: d})
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

package upnp

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap/buffer"

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
	AddIcon(Icon)
	AddService(*Service)

	DDDLocation() *url.URL
	UniqueDeviceName() string
	ConfigID() int32
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
	LastModified     time.Time `xml:"-"`
}

type device struct {
	DeviceSpec
	Icons    []Icon         `xml:"iconList>icon"`
	Services []*serviceDesc `xml:"serviceList>service"`

	router *mux.Router
	soap   *soap.Server
	logger logging.Logger
	sync.Mutex
}

type Icon struct {
	Mimetype string `xml:"mimetype"`
	URL      string `xml:"url"`
	Width    int    `xml:"width"`
	Height   int    `xml:"height"`
	Depth    int    `xml:"depth"`
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
		HeadersRegexp("Content-Type", `(application|text)/(soap\+)?xml`).
		Name(ControlRoute).
		Handler(dev.soap).
		GetError())

	must(router.Methods("GET").
		Path("/scpd/{service:[0-9]+}.xml").
		Name(SCPDRoute).
		HandlerFunc(dev.describeService).
		GetError())

	return dev
}

func (d *device) AddIcon(icon Icon) {
	d.Icons = append(d.Icons, icon)
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

	for _, urn := range ExpandTypes(s.urn) {
		for name, action := range s.actions {
			d.soap.RegisterAction(xml.Name{urn, name}, action)
		}
	}
}

func (d *device) DDDLocation() (res *url.URL) {
	res, err := d.router.Get(DDDRoute).URL()
	if err != nil {
		d.logger.DPanic("Could not build DDD URL: %s", err)
	}
	return
}

func (d *device) UniqueDeviceName() string {
	return d.UDN
}

func (d *device) ConfigID() int32 {
	return int32(d.LastModified.Unix() & 0x3fff)
}

var versionedTypeRe = regexp.MustCompile(`^(urn:schemas-upnp-org:(?:service|device):[^:]+:)(\d+)$`)

func ExpandTypes(t string) (ts []string) {
	subs := versionedTypeRe.FindStringSubmatch(t)
	if subs == nil {
		return []string{t}
	}
	v, err := strconv.Atoi(subs[2])
	if err != nil {
		log.Panicf("cannot convert %q to int: %s", subs[2], err)
	}
	for ; v >= 1; v-- {
		ts = append(ts, fmt.Sprintf("%s%d", subs[1], v))
	}
	return
}

func (d *device) DeviceTypes() []string {
	return []string{d.DeviceType}
}

func (d *device) ServiceTypes() (res []string) {
	for _, s := range d.Services {
		res = append(res, s.URN)
	}
	return
}

func (d *device) describeDevice(w http.ResponseWriter, r *http.Request) {
	urlBase := &url.URL{Scheme: "http", Host: r.Host, Path: ""}
	d.serveXML(w, r, rootDevice{
		SpecVersion: specVersion{1, 0},
		URLBase:     urlBase.String(),
		Device:      d,
	})
}

func (d *device) describeService(w http.ResponseWriter, r *http.Request) {
	idx, err := strconv.Atoi(mux.Vars(r)["service"])
	if err != nil {
		http.Error(w, "Unknown service", http.StatusNotFound)
		return
	}
	d.serveXML(w, r, d.Services[idx].service)
}

var bufferPool = buffer.NewPool()

func (d *device) serveXML(w http.ResponseWriter, r *http.Request, data interface{}) {
	b := bufferPool.Get()
	defer b.Free()

	_, err := b.Write([]byte(xml.Header))
	if err == nil {
		err = xml.NewEncoder(b).Encode(data)
	}
	if err != nil {
		d.logger.Error(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", `text/xml; encoding="utf-8"`)
	http.ServeContent(w, r, path.Base(r.URL.Path), d.LastModified, bytes.NewReader(b.Bytes()))
}

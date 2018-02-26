package upnp

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap/buffer"

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
	AddService(*Service) error

	DDDLocation() (*url.URL, error)
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

func NewDevice(spec DeviceSpec, router *mux.Router) (ret Device, err error) {
	dev := &device{
		DeviceSpec: spec,
		router:     router,
		soap:       soap.New(),
	}
	ret = dev

	err = router.Methods("GET").
		Path("/device.xml").
		Name(DDDRoute).
		HandlerFunc(dev.describeDevice).
		GetError()
	if err != nil {
		return
	}

	err = router.Methods("POST").
		Path("/control").
		HeadersRegexp("Content-Type", `(application|text)/(soap\+)?xml`).
		Name(ControlRoute).
		Handler(dev.soap).
		GetError()
	if err != nil {
		return
	}

	err = router.Methods("GET").
		Path("/scpd/{service:[0-9]+}.xml").
		Name(SCPDRoute).
		HandlerFunc(dev.describeService).
		GetError()

	return
}

func (d *device) AddIcon(icon Icon) {
	d.Icons = append(d.Icons, icon)
}

func (d *device) AddService(s *Service) (err error) {
	idx := len(d.Services)
	desc := &serviceDesc{ID: s.id, URN: s.urn, service: s, EventSubURL: "/sub"}
	d.Services = append(d.Services, desc)

	url, err := d.router.Get(ControlRoute).URLPath()
	if err != nil {
		return
	}
	desc.ControlURL = url.String()

	url, err = d.router.Get(SCPDRoute).URLPath("service", strconv.Itoa(idx))
	if err != nil {
		return
	}
	desc.SCPDURL = url.String()

	urns, err := ExpandTypes(s.urn)
	if err != nil {
		return
	}
	for _, urn := range urns {
		for name, action := range s.actions {
			err = d.soap.RegisterAction(xml.Name{urn, name}, action)
			if err != nil {
				return
			}
		}
	}

	return
}

func (d *device) DDDLocation() (res *url.URL, err error) {
	return d.router.Get(DDDRoute).URLPath()
}

func (d *device) UniqueDeviceName() string {
	return d.UDN
}

func (d *device) ConfigID() int32 {
	return int32(d.LastModified.Unix() & 0x3fff)
}

var versionedTypeRe = regexp.MustCompile(`^(urn:schemas-upnp-org:(?:service|device):[^:]+:)(\d+)$`)

func ExpandTypes(t string) (ts []string, err error) {
	subs := versionedTypeRe.FindStringSubmatch(t)
	if subs == nil {
		return []string{t}, nil
	}
	v, err := strconv.Atoi(subs[2])
	if err != nil {
		return
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", `text/xml; encoding="utf-8"`)
	http.ServeContent(w, r, path.Base(r.URL.Path), d.LastModified, bytes.NewReader(b.Bytes()))
}

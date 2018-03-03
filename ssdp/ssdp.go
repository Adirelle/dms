package ssdp

import (
	"net"
	"time"

	"github.com/anacrolix/dms/logging"
	"gopkg.in/thejerf/suture.v2"
)

const (
	AddrString = "239.255.255.250:1900"
	rootDevice = "upnp:rootdevice"
)

var NetAddr = &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 1900}

type Config struct {
	Interfaces     func() ([]net.Interface, error)
	Server         string
	Services       []string
	Devices        []string
	Location       func(net.IP) string
	UUID           string
	NotifyInterval time.Duration
	BootID         int32
	ConfigID       int32
}

func (c *Config) usnFromTarget(target string) string {
	if target == c.UUID {
		return target
	}
	return c.UUID + "::" + target
}

func (c *Config) allTypes() []string {
	return append(
		append([]string{rootDevice, c.UUID}, c.Devices...),
		c.Services...,
	)
}

type Service suture.Service

func New(c Config, l logging.Logger) Service {
	spv := suture.NewSimple("ssdp")
	r := NewResponder(c, l.Named("responder"))
	spv.Add(r)
	spv.Add(NewAdvertiser(c, r.Port, l.Named("advertiser")))
	return spv
}

func getIP(v interface{}) (net.IP, bool) {
	switch addr := v.(type) {
	case *net.IPAddr:
		return addr.IP, true
	case *net.IPNet:
		return addr.IP, true
	case *net.UDPAddr:
		return addr.IP, true
	case *net.TCPAddr:
		return addr.IP, true
	}
	return net.IP{}, false
}

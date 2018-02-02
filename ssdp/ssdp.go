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

type SSDPConfig struct {
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

func (c *SSDPConfig) usnFromTarget(target string) string {
	if target == c.UUID {
		return target
	}
	return c.UUID + "::" + target
}

func (c *SSDPConfig) allTypes() []string {
	return append(
		append([]string{rootDevice, c.UUID}, c.Devices...),
		c.Services...,
	)
}

func New(c SSDPConfig, l logging.Logger) suture.Service {
	l = l.Named("ssdp")
	spv := suture.NewSimple("ssdp")
	r := NewResponder(c, l)
	spv.Add(r)
	spv.Add(NewAdvertiser(c, r.Port, l))
	return spv
}

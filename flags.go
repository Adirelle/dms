package main

import (
	"fmt"
	"net"
	"strings"
)

type stringsVar []string

func (s stringsVar) String() string {
	return strings.Join([]string(s), ", ")
}

func (s stringsVar) Get() interface{} {
	return []string(s)
}

func (s stringsVar) Set(more string) error {
	s = append(s, more)
	return nil
}

type configFileVar struct{ c *Config }

func (c configFileVar) String() string {
	return ""
}

func (c configFileVar) Get() interface{} {
	return c.c
}

func (c configFileVar) Set(path string) error {
	err := c.c.load(path)
	if err != nil {
		return fmt.Errorf("Error loading configuration file (%s): %s", path, err.Error())
	}
	return nil
}

type tcpAddrVar struct{ addr *net.TCPAddr }

func (t tcpAddrVar) String() string {
	return t.addr.String()
}

func (t tcpAddrVar) Get() interface{} {
	return t.addr
}

func (t tcpAddrVar) Set(address string) (err error) {
	addr, err := net.ResolveTCPAddr("tcp", address)
	if err == nil {
		t.addr.IP = addr.IP
		t.addr.Port = addr.Port
		t.addr.Zone = addr.Zone
	}
	return
}

type Interface struct{ iface *net.Interface }

func (i *Interface) String() string {
	if i.iface != nil {
		return i.iface.Name
	}
	return ""
}

func (i *Interface) Get() interface{} {
	return i.iface
}

func (i *Interface) Set(ifname string) (err error) {
	iface, err := net.InterfaceByName(ifname)
	if err == nil {
		i.iface = iface
	}
	return
}

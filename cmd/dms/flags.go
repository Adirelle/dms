package main

import (
	"fmt"
	"net"
)

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
		return fmt.Errorf("in configuration file: %s", err)
	}
	return nil
}

type tcpAddrVar struct{ Addr *net.TCPAddr }

func (t *tcpAddrVar) String() string {
	return t.Addr.String()
}

func (t *tcpAddrVar) Get() interface{} {
	return t.Addr
}

func (t *tcpAddrVar) Set(address string) (err error) {
	t.Addr, err = net.ResolveTCPAddr("tcp", address)
	return
}

func (t *tcpAddrVar) MarshalText() ([]byte, error) {
	return []byte(t.String()), nil
}

func (t *tcpAddrVar) UnmarshalText(b []byte) error {
	if addrStr := string(b); addrStr != "" {
		return t.Set(addrStr)
	}
	return nil
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
	} else {
		err = fmt.Errorf("%q: %s", ifname, err)
	}
	return
}

func (i *Interface) MarshalText() ([]byte, error) {
	return []byte(i.String()), nil
}

func (i *Interface) UnmarshalText(b []byte) error {
	if ifname := string(b); ifname != "" {
		return i.Set(ifname)
	}
	return nil
}

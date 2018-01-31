package main

import (
	"encoding/json"
	"net"
	"os"
	"time"

	"github.com/anacrolix/dms/dlna/dms"
	"github.com/anacrolix/dms/logging"
)

type dmsConfig struct {
	dms.Config
	Logging          logging.Config
	Path             string
	IfName           string
	Http             string
	FFprobeCachePath string
	NoProbe          bool
	NotifyInterval   time.Duration
}

func (c *dmsConfig) load(configPath string) (err error) {
	file, err := os.Open(configPath)
	if err != nil {
		return
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	return decoder.Decode(&c)
}

func (c *dmsConfig) Interfaces() ([]net.Interface, error) {
	if c.IfName == "" {
		return net.Interfaces()
	}
	iface, err := net.InterfaceByName(c.IfName)
	return []net.Interface{*iface}, err
}

func (c *dmsConfig) ValidInterfaces() (ret []net.Interface, err error) {
	ifaces, err := c.Interfaces()
	if err != nil {
		return
	}
	ret = make([]net.Interface, 0, len(ifaces))
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp != 0 && iface.MTU > 0 {
			ret = append(ret, iface)
		}
	}
	return
}

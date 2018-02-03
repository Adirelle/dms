package main

import (
	"flag"
	"fmt"
	"net"
	"strings"
	"time"
)

func setupFlags(config *dmsConfig) {

	flag.Var(configFileVar{config}, "config", "json configuration file")

	flag.StringVar(&config.RootObjectPath, "path", ".", "browse root path")
	flag.Var(tcpAddrVar{config.HTTP}, "http", "http server port")
	flag.Var(ifaceVar{&config.Interface}, "ifname", "network interface to bind to")
	flag.StringVar(&config.FriendlyName, "friendlyName", "", "server friendly name")
	flag.StringVar(&config.FFprobeCachePath, "fFprobeCachePath", getDefaultFFprobeCachePath(), "path to FFprobe cache file")

	flag.DurationVar(&config.NotifyInterval, "notifyInterval", 30*time.Minute, "interval between SSPD announces")

	flag.BoolVar(&config.LogHeaders, "logHeaders", false, "log HTTP headers")
	flag.BoolVar(&config.NoTranscode, "noTranscode", false, "disable transcoding")
	flag.BoolVar(&config.NoProbe, "noProbe", false, "disable media probing with ffprobe")
	flag.BoolVar(&config.StallEventSubscribe, "stallEventSubscribe", false, "workaround for some bad event subscribers")
	flag.BoolVar(&config.IgnoreHidden, "ignoreHidden", false, "ignore hidden files and directories")
	flag.BoolVar(&config.IgnoreUnreadable, "ignoreUnreadable", false, "ignore unreadable files and directories")

	flag.BoolVar(&config.Logging.Debug, "debug", false, "Enable development logging")
	flag.Var(stringsVar(config.Logging.OutputPaths), "logPath", "Log files")
	flag.Var(&config.Logging.Level, "logLevel", "Minimum log level")
	flag.BoolVar(&config.Logging.NoDate, "logNoDate", false, "Disable timestamp in log")

}

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

type configFileVar struct{ c *dmsConfig }

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

type ifaceVar struct{ iface **net.Interface }

func (i ifaceVar) String() string {
	if *i.iface != nil {
		return (*i.iface).Name
	}
	return ""
}

func (i ifaceVar) Get() interface{} {
	return i.iface
}

func (i ifaceVar) Set(ifname string) (err error) {
	iface, err := net.InterfaceByName(ifname)
	if err == nil {
		*i.iface = iface
	}
	return
}

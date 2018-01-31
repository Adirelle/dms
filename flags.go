package main

import (
	"flag"
	"fmt"
	"strings"
	"time"
)

func setupFlags(config *dmsConfig) {

	flag.Var(configFileVar{config}, "config", "json configuration file")

	flag.StringVar(&config.Path, "path", ".", "browse root path")
	flag.StringVar(&config.Http, "http", ":1338", "http server port")
	flag.StringVar(&config.IfName, "ifname", "", "specific SSDP network interface")
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

package main

//go:generate go-bindata data/

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/anacrolix/dms/dlna/dms"
	"github.com/anacrolix/dms/ffmpeg"
	"github.com/anacrolix/dms/logging"
	"github.com/anacrolix/dms/rrcache"
	"github.com/anacrolix/dms/ssdp"
	"gopkg.in/thejerf/suture.v2"
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

func main() {
	config := &dmsConfig{Logging: logging.DefaultConfig()}

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

	flag.Parse()
	if flag.NArg() != 0 {
		flag.Usage()
		log.Printf("%s: %s\n", "unexpected positional arguments", flag.Args())
	}

	logger := logging.New(config.Logging)
	defer logger.Sync()

	var ffProber ffmpeg.FFProber
	if config.NoProbe {
		ffProber = ffmpeg.NewFFProber(true, nil)
	} else {
		cache := &fFprobeCache{
			c: rrcache.New(64 << 20),
		}
		if err := cache.load(config.FFprobeCachePath); err == nil {
			logger.Warnf("could load cache: %s", err.Error())
		}
		defer func() {
			if err := cache.save(config.FFprobeCachePath); err != nil {
				logger.Warnf("could not save cache: %s", err.Error())
			}
		}()

		ffProber = ffmpeg.NewFFProber(false, cache)
	}

	httpServer := makeHTTPServer(config, ffProber, logger)

	spv := suture.New("dms", suture.Spec{Log: func(msg string) { logger.Warn(msg) }})
	spv.ServeBackground()
	defer spv.Stop()

	spv.Add(httpServer)
	spv.Add(ssdp.New(makeSSDPConfig(config, httpServer), logger))

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	<-sigs
}

func makeHTTPServer(config *dmsConfig, ffProber ffmpeg.FFProber, l logging.Logger) *dms.Server {
	ifaces, _ := config.ValidInterfaces()
	return &dms.Server{
		Config:     config.Config,
		Interfaces: ifaces,
		HTTPConn: func() net.Listener {
			conn, err := net.Listen("tcp", config.Http)
			if err != nil {
				log.Fatal(err)
			}
			return conn
		}(),
		Icons: []dms.Icon{
			dms.Icon{
				Width:      48,
				Height:     48,
				Depth:      8,
				Mimetype:   "image/png",
				ReadSeeker: bytes.NewReader(MustAsset("data/VGC Sonic.png")),
			},
			dms.Icon{
				Width:      128,
				Height:     128,
				Depth:      8,
				Mimetype:   "image/png",
				ReadSeeker: bytes.NewReader(MustAsset("data/VGC Sonic 128.png")),
			},
		},
		FFProber: ffProber,
		L:        l.Named("http"),
	}
}

func makeSSDPConfig(config *dmsConfig, httpServer *dms.Server) ssdp.SSDPConfig {
	return ssdp.SSDPConfig{
		NotifyInterval: config.NotifyInterval,
		Interfaces:     config.ValidInterfaces,
		Location:       httpServer.DDDLocation,
		Server:         dms.ServerToken,
		Services:       httpServer.ServiceTypes(),
		Devices:        httpServer.Devices(),
		UUID:           httpServer.DeviceUUID(),
		BootID:         httpServer.GetBootID(),
		ConfigID:       httpServer.GetConfigID(),
	}
}

func getDefaultFFprobeCachePath() (path string) {
	_user, err := user.Current()
	if err != nil {
		log.Print(err)
		return
	}
	path = filepath.Join(_user.HomeDir, ".dms-ffprobe-cache")
	return
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

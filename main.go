package main

//go:generate go-bindata data/

import (
	"context"
	"net/http"

	"github.com/anacrolix/dms/content_directory"

	"github.com/satori/go.uuid"

	"encoding/json"
	"flag"
	"fmt"
	"hash/crc32"
	"log"
	"net"
	"os"
	"os/signal"
	"os/user"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/anacrolix/dms/filesystem"
	"github.com/anacrolix/dms/logging"
	"github.com/anacrolix/dms/ssdp"
	"github.com/anacrolix/dms/upnp"
	"github.com/gorilla/mux"
	"gopkg.in/thejerf/suture.v2"
)

var ServerToken = fmt.Sprintf("%s/1.0 DLNADOC/1.50 UPnP/2.0 DMS/1.0", strings.Title(runtime.GOOS))

const (
	DeviceType       = "urn:schemas-upnp-org:device:MediaServer:1"
	Manufacturer     = "Matt Joiner <anacrolix@gmail.com>"
	ManufacturerURL  = "http://github.com/anacrolx"
	ModelDescription = "Open-source Digital Media Server written in Go !"
	ModelName        = "dms 1.0"
	ModelNumber      = 1
	ModelURL         = "http://github.com/anacrolix/dms"
)

func main() {
	config := &Config{
		FriendlyName:   getDefaultFriendlyName(),
		Config:         filesystem.Config{Root: "."},
		NotifyInterval: 30 * time.Minute,
		HTTP:           &net.TCPAddr{Port: 1338},
		Logging:        logging.DefaultConfig(),
	}

	config.ParseArgs()

	ctn := Container{Config: config}

	spv := ctn.Supervisor()

	ctn.Logger().Infof("DMS #%s, build on %s", CommitRef, BuildDate)

	spv.ServeBackground()

	defer func() {
		spv.Stop()
		ctn.Logger().Sync()
	}()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	<-sigs
}

type Config struct {
	FriendlyName string
	filesystem.Config
	Logging   logging.Config
	Interface *net.Interface
	HTTP      *net.TCPAddr
	// FFprobeCachePath string
	// NoProbe          bool
	NotifyInterval time.Duration
}

func (c *Config) SetupFlags() {
	flag.Var(configFileVar{c}, "config", "json configuration file")

	flag.StringVar(&c.Root, "path", c.Root, "browse root path")
	flag.Var(tcpAddrVar{c.HTTP}, "http", "http server port")
	flag.Var(ifaceVar{&c.Interface}, "ifname", "network interface to bind to")
	flag.StringVar(&c.FriendlyName, "friendlyName", c.FriendlyName, "server friendly name")
	// flag.StringVar(&config.FFprobeCachePath, "fFprobeCachePath", getDefaultFFprobeCachePath(), "path to FFprobe cache file")

	flag.DurationVar(&c.NotifyInterval, "notifyInterval", c.NotifyInterval, "interval between SSPD announces")

	// flag.BoolVar(&config.LogHeaders, "logHeaders", false, "log HTTP headers")
	// flag.BoolVar(&config.NoTranscode, "noTranscode", false, "disable transcoding")
	// flag.BoolVar(&config.NoProbe, "noProbe", false, "disable media probing with ffprobe")
	// flag.BoolVar(&config.StallEventSubscribe, "stallEventSubscribe", false, "workaround for some bad event subscribers")
	flag.BoolVar(&c.IgnoreHidden, "ignoreHidden", c.IgnoreHidden, "ignore hidden files and directories")
	flag.BoolVar(&c.IgnoreUnreadable, "ignoreUnreadable", c.IgnoreUnreadable, "ignore unreadable files and directories")
	// flag.StringVar(&config.AccessLogPath, "accessLogPath", "", "path to log HTTP requests")

	flag.BoolVar(&c.Logging.Debug, "debug", c.Logging.Debug, "Enable development logging")
	flag.Var(stringsVar(c.Logging.OutputPaths), "logPath", "Log files")
	flag.Var(&c.Logging.Level, "logLevel", "Minimum log level")
	flag.BoolVar(&c.Logging.NoDate, "logNoDate", c.Logging.NoDate, "Disable timestamp in log")
}

func (c *Config) ParseArgs() {
	c.SetupFlags()

	flag.Parse()
	if flag.NArg() != 0 {
		flag.Usage()
		log.Fatalf("%s: %s\n", "unexpected positional arguments", flag.Args())
	}
}

func (c *Config) load(configPath string) (err error) {
	file, err := os.Open(configPath)
	if err != nil {
		return
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	return decoder.Decode(&c)
}

func (c *Config) CRC32() uint32 {
	hash := crc32.NewIEEE()
	json.NewEncoder(hash).Encode(c)
	return hash.Sum32()
}

func getDefaultFriendlyName() string {
	username := "nobody"
	if user, err := user.Current(); err == nil {
		username = user.Name
	}
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "localhost"
	}
	return fmt.Sprintf("%s: %s on %s", ModelName, username, hostname)
}

type Container struct {
	*Config

	router    *mux.Router
	upnp      upnp.Device
	fs        filesystem.Filesystem
	logger    logging.Logger
	udn       string
	ssdp      suture.Service
	http      suture.Service
	spv       *suture.Supervisor
	cd        *content_directory.Service
	cdBackend content_directory.Backend
}

func (c *Container) Supervisor() *suture.Supervisor {
	if c.spv == nil {
		logger := c.Logger().Named("supervisor")
		c.spv = suture.New("dms", suture.Spec{Log: func(m string) { logger.Warn(m) }})
		c.spv.Add(c.HTTPService())
		c.spv.Add(c.SSDPService())
	}
	return c.spv
}

func (c *Container) Logger() logging.Logger {
	if c.logger == nil {
		c.logger = logging.New(c.Logging)
	}
	return c.logger
}

func (c *Container) HTTPService() suture.Service {
	if c.http == nil {
		c.http = &httpWrapper{
			http.Server{Addr: c.HTTP.String(), Handler: c.Router()},
			c.Logger().Named("http"),
		}
	}
	return c.http
}

func (c *Container) Router() *mux.Router {
	if c.router == nil {
		c.router = mux.NewRouter()
	}
	return c.router
}

func (c *Container) SSDPService() suture.Service {
	if c.ssdp == nil {
		upnp := c.UPNP()
		c.ssdp = ssdp.New(
			ssdp.Config{
				NotifyInterval: c.NotifyInterval,
				Interfaces:     c.ValidInterfaces,
				Server:         ServerToken,
				Location: func(ip net.IP) string {
					url := upnp.DDDLocation()
					url.Scheme = "http"
					url.Host = fmt.Sprintf("%s:%d", ip, c.HTTP.Port)
					return url.String()
				},
				UUID:     c.UDN(),
				Devices:  upnp.DeviceTypes(),
				Services: upnp.ServiceTypes(),
				BootID:   int32(time.Now().Unix() & 0x3fff), // TODO find the right mask
				ConfigID: int32(c.CRC32()) & 0x3fff,         // TODO find the right mask
			},
			c.Logger().Named("ssdp"),
		)
	}
	return c.ssdp
}

func (c *Container) UPNP() upnp.Device {
	if c.upnp == nil {
		c.upnp = upnp.NewDevice(
			upnp.DeviceSpec{
				DeviceType:       DeviceType,
				FriendlyName:     c.FriendlyName,
				Manufacturer:     Manufacturer,
				ManufacturerURL:  ManufacturerURL,
				ModelDescription: ModelDescription,
				ModelName:        ModelName,
				ModelNumber:      ModelNumber,
				ModelURL:         ModelURL,
				UDN:              c.UDN(),
				UPC:              "000000",
			},
			c.Router(),
			c.Logger().Named("upnp"),
		)
		c.upnp.AddService(c.ContentDirectory().UPNPService())
		c.upnp.AddIcon(48, 48, 32, "image/png", MustAsset("data/VGC Sonic.png"))
		c.upnp.AddIcon(128, 128, 32, "image/png", MustAsset("data/VGC Sonic 128.png"))
	}
	return c.upnp
}

func (c *Container) UDN() string {
	if c.udn == "" {
		c.udn = "uuid:" + uuid.NewV5(uuid.NamespaceX500, c.FriendlyName).String()
	}
	return c.udn
}

func (c *Container) ContentDirectory() *content_directory.Service {
	if c.cd == nil {
		c.cd = content_directory.NewService(c.CDBackend(), c.Filesystem(), c.Logger().Named("cd"))
	}
	return c.cd
}

func (c *Container) CDBackend() content_directory.Backend {
	if c.cdBackend == nil {
		c.cdBackend = content_directory.NewSimpleBackend(c.Logger().Named("cd-backend"))
	}
	return c.cdBackend
}

func (c *Container) Filesystem() filesystem.Filesystem {
	if c.fs == nil {
		var err error
		c.fs, err = filesystem.New(c.Config.Config)
		if err != nil {
			c.Logger().Panic(err)
		}
	}
	return c.fs
}

func (c *Container) ValidInterfaces() (ret []net.Interface, err error) {
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

func (c *Container) Interfaces() ([]net.Interface, error) {
	if c.Interface == nil {
		return net.Interfaces()
	}
	return []net.Interface{*c.Interface}, nil
}

type httpWrapper struct {
	http.Server
	logging.Logger
}

func (w *httpWrapper) Serve() {
	w.Infof("listening on %s", w.Addr)
	err := w.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		w.Error(err)
	}
}

func (w *httpWrapper) Stop() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := w.Shutdown(ctx)
	if err != nil {
		w.Error(err)
	}
	w.Info("stopped")
}

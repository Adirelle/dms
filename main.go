package main

//go:generate go-bindata data/

import (
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
	DeviceType   = "urn:schemas-upnp-org:device:MediaServer:1"
	Manufacturer = "Matt Joiner <anacrolix@gmail.com>"
	ModelName    = "dms 1.0"
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
	defer config.Logger().Sync()

	spv := suture.New("dms", suture.Spec{Log: func(msg string) { config.Logger().Warn(msg) }})
	defer spv.Stop()

	spv.Add(config.HTTPService())
	spv.Add(config.SSDPService())
	spv.ServeBackground()

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

	router *mux.Router
	upnp   upnp.Device
	fs     filesystem.Filesystem
	logger logging.Logger
	udn    string
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

func (c *Config) Logger() logging.Logger {
	if c.logger == nil {
		c.logger = logging.New(c.Logging)
	}
	return c.logger
}

func (c *Config) Filesystem() filesystem.Filesystem {
	if c.fs == nil {
		var err error
		c.fs, err = filesystem.New(c.Config)
		if err != nil {
			c.Logger().Panic(err)
		}
	}
	return c.fs
}

func (c *Config) Interfaces() ([]net.Interface, error) {
	if c.Interface == nil {
		return net.Interfaces()
	}
	return []net.Interface{*c.Interface}, nil
}

func (c *Config) ValidInterfaces() (ret []net.Interface, err error) {
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

func (c *Config) Router() *mux.Router {
	if c.router == nil {
		c.router = mux.NewRouter()
	}
	return c.router
}

func (c *Config) UDN() string {
	if c.udn == "" {
		c.udn = "uuid:" + uuid.NewV5(uuid.NamespaceX500, c.FriendlyName).String()
	}
	return c.udn
}

func (c *Config) UPNP() upnp.Device {
	if c.upnp == nil {
		c.upnp = upnp.NewDevice(
			DeviceType,
			c.FriendlyName,
			Manufacturer,
			ModelName,
			c.UDN(),
			c.Router(),
			c.Logger(),
		)
	}
	return c.upnp
}

func (c *Config) HTTPService() suture.Service {
	return nil
}

func (c *Config) SSDPService() suture.Service {
	return ssdp.New(
		ssdp.Config{
			NotifyInterval: c.NotifyInterval,
			Interfaces:     c.ValidInterfaces,
			Server:         ServerToken,
			Location:       nil,
			UUID:           c.UDN(),
			Devices:        []string{DeviceType},
			Services:       []string{content_directory.ServiceType},
			BootID:         int32(time.Now().Unix() & 0x3fff), // TODO find the right mask
			ConfigID:       int32(c.CRC32()) & 0x3fff,         // TODO find the right mask
		},
		c.Logger(),
	)
}

func (c *Config) CRC32() uint32 {
	hash := crc32.NewIEEE()
	json.NewEncoder(hash).Encode(*c)
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

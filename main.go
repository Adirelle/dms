package main

//go:generate bash ./versionInfo.sh version.go
//go:generate go generate assets/fs.go

import (
	"io"
	"net/http"

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

	"github.com/anacrolix/dms/assets"
	"github.com/anacrolix/dms/cds"
	"github.com/anacrolix/dms/filesystem"
	dmsHttp "github.com/anacrolix/dms/http"
	"github.com/anacrolix/dms/logging"
	"github.com/anacrolix/dms/processor"
	"github.com/anacrolix/dms/rest"
	"github.com/anacrolix/dms/ssdp"
	"github.com/anacrolix/dms/upnp"
	"github.com/bluele/gcache"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	"github.com/satori/go.uuid"
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
		Interface:      Interface{},
		NotifyInterval: 30 * time.Minute,
		HTTP:           &net.TCPAddr{Port: 1338},
		Logging:        logging.DefaultConfig(),
	}

	config.ParseArgs()

	ctn := Container{Config: config}

	ctn.Logger("main").Infof("DMS #%s, build on %s", CommitRef, BuildDate)

	spv := ctn.Supervisor()

	spv.ServeBackground()
	defer spv.Stop()

	defer ctn.Logger("").Sync()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	<-sigs
}

type Config struct {
	FriendlyName string
	filesystem.Config
	Logging   logging.Config
	Interface Interface
	HTTP      *net.TCPAddr
	AccessLog string
	// FFprobeCachePath string
	// NoProbe          bool
	NotifyInterval time.Duration
	Debug          bool
}

func (c *Config) SetupFlags() {
	flag.Var(configFileVar{c}, "config", "json configuration file")

	flag.StringVar(&c.Root, "path", c.Root, "browse root path")
	flag.Var(tcpAddrVar{c.HTTP}, "http", "http server port")
	flag.Var(&c.Interface, "ifname", "network interface to bind to")
	flag.StringVar(&c.FriendlyName, "friendlyName", c.FriendlyName, "server friendly name")

	flag.DurationVar(&c.NotifyInterval, "notifyInterval", c.NotifyInterval, "interval between SSPD announces")

	flag.StringVar(&c.AccessLog, "accessLog", "", "path to log HTTP requests")
	flag.BoolVar(&c.Debug, "debug", c.Debug, "enable debugging features")

	flag.Var(&c.Logging.Level, "level", "set logging levels")
	flag.BoolVar(&c.Logging.Quiet, "quiet", c.Logging.Quiet, "only show errors")
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

	router        *mux.Router
	upnp          upnp.Device
	fs            *filesystem.Filesystem
	loggerFactory *logging.Factory
	udn           string
	ssdp          suture.Service
	http          suture.Service
	spv           *suture.Supervisor
	cds           *cds.Service
	directory     cds.ContentDirectory
	fileserver    *cds.FileServer
	accessLog     io.Writer

	indent string
}

func (c *Container) Supervisor() *suture.Supervisor {
	if c.spv == nil {
		defer c.creating("Supervisor")()
		logger := c.Logger("supervisor")
		c.spv = suture.New("dms", suture.Spec{Log: func(m string) { logger.Warn(m) }})
		c.spv.Add(c.HTTPService())
		c.spv.Add(c.SSDPService())
	}
	return c.spv
}

func (c *Container) Logger(name string) logging.Logger {
	if c.loggerFactory == nil {
		c.Logging.Debug = c.Debug
		c.loggerFactory = c.Logging.Build()
	}
	return c.loggerFactory.Get(name)
}

func (c *Container) HTTPService() suture.Service {
	if c.http == nil {
		defer c.creating("HTTP Service")()
		logger := c.Logger("http")
		stdLogger, err := logger.StdLoggerAt(logging.ErrorLevel)
		if err != nil {
			c.Logger("container").Fatalf("cannot initialize the http logger: %s", err)
		}
		server := http.Server{
			Addr:     c.HTTP.String(),
			Handler:  c.Router(),
			ErrorLog: stdLogger,
		}
		c.http = &dmsHttp.Service{server, logger}
	}
	return c.http
}

func (c *Container) Router() *mux.Router {
	if c.router == nil {
		defer c.creating("Router")()
		c.router = mux.NewRouter()
		c.SetupRouting(c.router)
		c.SetupMiddlewares(c.router)
	}
	return c.router
}

func (c *Container) SetupRouting(r *mux.Router) {
	defer c.creating("Routing")()

	if c.Debug {
		r.Methods("GET").Path("/debug/router").Handler(&dmsHttp.RouterDebug{r, c.Logger("router-debug")})
	}

	r.Methods("GET", "HEAD").PathPrefix("/icons/").
		Handler(http.FileServer(assets.FileSystem))

	r.Methods("GET").PathPrefix("/rest/").
		Handler(rest.New("/rest", c.ContentDirectory(), c.Logger("rest")))

	r.Methods("GET", "HEAD").PathPrefix("/files/").
		Handler(c.FileServer())

	r.Methods("GET", "HEAD").Path("/").
		Handler(http.RedirectHandler("/rest/", http.StatusSeeOther))
}

func (c *Container) SetupMiddlewares(r *mux.Router) {
	defer c.creating("Middleware")()

	r.Use(logging.AddLogger(c.Logger("http.request")))
	r.Use(dmsHttp.UniqueID)
	r.Use(dmsHttp.DebugRequest)

	accessLog := c.AccessLog()
	if accessLog != nil {
		r.Use(func(next http.Handler) http.Handler {
			return handlers.LoggingHandler(accessLog, next)
		})
	}

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Server", ServerToken)
			w.Header().Set("Ext", "")
			next.ServeHTTP(w, r)
		})
	})

}

func (c *Container) AccessLog() io.Writer {
	if c.accessLog == nil && c.Config.AccessLog != "" {
		defer c.creating("Access log")()
		fpath := c.Config.AccessLog
		if fpath == "-" {
			c.accessLog = os.Stdout
		} else {
			var err error
			c.accessLog, err = os.OpenFile(fpath, os.O_CREATE|os.O_APPEND, 0644)
			if err != nil {
				c.Logger("main").Fatalf("cannot open access log file: %s", err)
			}
		}
	}
	return c.accessLog
}

func (c *Container) SSDPService() suture.Service {
	if c.ssdp == nil {
		defer c.creating("SSDP Service")()
		upnp := c.UPNP()
		c.ssdp = ssdp.New(
			ssdp.Config{
				NotifyInterval: c.NotifyInterval,
				Interfaces:     c.ValidInterfaces,
				Server:         ServerToken,
				Location: func(ip net.IP) string {
					url, err := upnp.DDDLocation()
					if err != nil {
						panic(err)
					}
					return fmt.Sprintf("http://%s:%d%s", ip, c.HTTP.Port, url)
				},
				UUID:     upnp.UniqueDeviceName(),
				Devices:  upnp.DeviceTypes(),
				Services: upnp.ServiceTypes(),
				BootID:   int32(time.Now().Unix() & 0x3fff), // TODO find the right mask
				ConfigID: upnp.ConfigID(),
			},
			c.Logger("ssdp"),
		)
	}
	return c.ssdp
}

func (c *Container) UPNP() upnp.Device {
	if c.upnp == nil {
		defer c.creating("UPNP Device")()
		svc, err := upnp.NewDevice(
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
				LastModified:     time.Unix(BuildDateUnixTS, 0),
			},
			c.Router(),
		)
		if err != nil {
			c.Logger("container").Panic(err)
		}
		c.upnp = svc
		err = svc.AddService(c.CDS().UPNPService())
		if err != nil {
			c.Logger("container").Panic(err)
		}
		svc.AddIcon(upnp.Icon{"image/png", "/icons/md.png", 48, 48, 32})
		svc.AddIcon(upnp.Icon{"image/png", "/icons/lg.png", 128, 128, 32})
	}
	return c.upnp
}

func (c *Container) UDN() string {
	if c.udn == "" {
		c.udn = "uuid:" + uuid.NewV5(uuid.NamespaceX500, c.FriendlyName).String()
	}
	return c.udn
}

func (c *Container) CDS() *cds.Service {
	if c.cds == nil {
		defer c.creating("ContentDirectoryService")()
		c.cds = cds.NewService(c.ContentDirectory())
	}
	return c.cds
}

func (c *Container) ContentDirectory() cds.ContentDirectory {
	if c.directory == nil {
		defer c.creating("ContentDirectory")()
		base := cds.NewFilesystemContentDirectory(c.Filesystem(), c.Logger("directory"))
		processing := &cds.ProcessingDirectory{ContentDirectory: base, Logger: c.Logger("processing")}
		c.directory = cds.NewCache(
			processing,
			gcache.New(1000).ARC(),
			c.Logger("cache"),
		)
		c.SetupProcessors(processing, c.directory)
	}
	return c.directory
}

func (c *Container) Filesystem() *filesystem.Filesystem {
	if c.fs == nil {
		defer c.creating("Filesystem")()
		var err error
		c.fs, err = filesystem.New(c.Config.Config)
		if err != nil {
			c.Logger("container").Fatalf("%s: %s", c.Config.Config.Root, err)
		}
		root, err := c.fs.Get(filesystem.RootID)
		if err != nil {
			c.Logger("container").Fatalf("%s: %s", c.Config.Config.Root, err)
		}
		c.Logger("main").Infof("serving content from %s", root.FilePath)
	}
	return c.fs
}

func (c *Container) SetupProcessors(d *cds.ProcessingDirectory, cache cds.ContentDirectory) {
	defer c.creating("Processors")()

	d.AddProcessor(100, c.FileServer())
	d.AddProcessor(95, &processor.AlbumArtProcessor{cache, c.FileServer(), c.Logger("album-art")})
	d.AddProcessor(90, &processor.BasicIconProcessor{})

	l := c.Logger("ffprobe")
	ffprober, err := processor.NewFFProbeProcessor("ffprobe", l)
	if err == nil {
		d.AddProcessor(80, ffprober)
	} else {
		l.Errorf("cannot initialize ffprobe: %s", err.Error())
	}
}

func (c *Container) FileServer() *cds.FileServer {
	if c.fileserver == nil {
		defer c.creating("FileServer")()
		c.fileserver = cds.NewFileServer(c.ContentDirectory(), "/files", c.Logger("fileserver"))
	}
	return c.fileserver
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
	iface := c.Interface.Get().(*net.Interface)
	if iface == nil {
		return net.Interfaces()
	}
	return []net.Interface{*iface}, nil
}

const indent = "│  "

func (c *Container) creating(what string) func() {
	l := c.Logger("container")
	l.Debugf("%s├─Creating %s", c.indent, what)
	c.indent += indent
	return func() {
		l.Debugf("%s└─%s created", c.indent, what)
		c.indent = c.indent[:len(c.indent)-len(indent)]
	}
}

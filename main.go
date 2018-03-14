package main

//go:generate bash ./versionInfo.sh version.go
//go:generate go generate assets/fs.go

import (
	"encoding/json"
	"flag"
	"fmt"
	"hash/crc32"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"os/user"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/Adirelle/go-libs/dic"
	adi_http "github.com/Adirelle/go-libs/http"
	"github.com/Adirelle/go-libs/logging"
	"github.com/anacrolix/dms/assets"
	"github.com/anacrolix/dms/cds"
	"github.com/anacrolix/dms/filesystem"
	"github.com/anacrolix/dms/processor"
	"github.com/anacrolix/dms/rest"
	"github.com/anacrolix/dms/ssdp"
	"github.com/anacrolix/dms/upnp"
	"github.com/gorilla/handlers"
	"github.com/gorilla/mux"
	uuid "github.com/satori/go.uuid"
	suture "gopkg.in/thejerf/suture.v2"
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
		FFProbe: processor.FFProbeConfig{
			BinPath:   "ffprobe",
			CacheSize: 10000,
			CacheTTL:  time.Minute,
			Limit:     20,
		},
		Cache: cds.CacheConfig{
			Size:       10000,
			SuccessTTL: time.Minute,
			FailureTTL: 10 * time.Second,
		},
	}

	config.ParseArgs()

	config.Logging.Debug = config.Debug
	lf := config.Logging.Build()
	l := lf.Get("")
	defer l.Sync()

	l.Infof("DMS #%s, build on %s", CommitRef, BuildDate)

	ctn := dic.New()
	inner := &Container{config, lf.Get("container"), lf}

	ctnLogger, err := inner.Logger.StdLoggerAt(logging.DebugLevel)
	if err != nil {
		l.Fatal(err)
	}
	ctn.LogTo(ctnLogger)

	ctn.RegisterFrom(inner)

	var spv *suture.Supervisor
	if err = ctn.Fetch(&spv); err != nil {
		l.Fatal(err)
	}
	spv.ServeBackground()
	defer spv.Stop()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	<-sigs
}

type Config struct {
	FriendlyName string
	filesystem.Config
	Logging        logging.Config
	Interface      Interface
	HTTP           *net.TCPAddr
	AccessLog      string
	NotifyInterval time.Duration
	Debug          bool
	FFProbe        processor.FFProbeConfig
	Cache          cds.CacheConfig
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

func (c *Config) Interfaces() ([]net.Interface, error) {
	iface := c.Interface.Get().(*net.Interface)
	if iface == nil {
		return net.Interfaces()
	}
	return []net.Interface{*iface}, nil
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
	Config        *Config
	Logger        logging.Logger
	LoggerFactory *logging.Factory
}

func (c *Container) logger(name string) logging.Logger {
	return c.LoggerFactory.Get(name)
}

type SSDPService suture.Service

func (c *Container) Supervisor(httpServer *adi_http.Service, ssdpServer ssdp.Service) *suture.Supervisor {
	l := c.logger("supervisor")
	spv := suture.New("dms", suture.Spec{Log: func(m string) { l.Warn(m) }})
	spv.Add(httpServer)
	spv.Add(ssdpServer)
	return spv
}

func (c *Container) HTTPService(r *mux.Router) *adi_http.Service {
	l := c.logger("http")
	stdLogger, err := l.StdLoggerAt(logging.ErrorLevel)
	if err != nil {
		c.logger("container").Fatalf("cannot initialize the http logger: %s", err)
	}
	server := http.Server{
		Addr:     c.Config.HTTP.String(),
		Handler:  r,
		ErrorLog: stdLogger,
	}
	return &adi_http.Service{server, l}
}

type AccessLog io.Writer

func (c *Container) Router(
	cd cds.ContentDirectory,
	fserver *cds.FileServer,
	al AccessLog,
) (r *mux.Router, err error) {
	r = mux.NewRouter()

	if c.Config.Debug {
		err = r.Methods("GET").Path("/debug/router").
			Handler(&adi_http.RouterDebug{r}).
			GetError()
		if err != nil {
			return
		}
	}

	err = r.Methods("GET", "HEAD").Path("/icons/" + processor.RouteIconTemplate + ".png").
		Name(processor.IconRoute).
		Handler(http.FileServer(assets.FileSystem)).
		GetError()
	if err != nil {
		return
	}

	err = r.Methods("GET").Path("/rest" + cds.RouteObjectIDTemplate).
		Name(rest.RouteName).
		Handler(rest.New(cd)).
		GetError()
	if err != nil {
		return
	}

	err = r.Methods("GET", "HEAD").Path("/files" + cds.RouteObjectIDTemplate).
		Name(cds.FileServerRoute).
		Handler(fserver).
		GetError()
	if err != nil {
		return
	}

	err = r.Methods("GET", "HEAD").Path("/").
		Handler(http.RedirectHandler("/rest/", http.StatusSeeOther)).
		GetError()
	if err != nil {
		return
	}

	r.Use(logging.AddLogger(c.logger("")))
	r.Use(adi_http.UniqueID)
	r.Use(adi_http.DebugRequest)
	r.Use(adi_http.AddURLGenerator(r))

	if al != nil {
		r.Use(func(next http.Handler) http.Handler {
			return handlers.LoggingHandler(al, next)
		})
	}

	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Server", ServerToken)
			w.Header().Set("Ext", "")
			next.ServeHTTP(w, r)
		})
	})

	return
}

func (c *Container) AccessLog() (al AccessLog) {
	fpath := c.Config.AccessLog
	if fpath == "-" {
		al = os.Stdout
	} else if fpath != "" {
		var err error
		al, err = os.OpenFile(fpath, os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			c.Logger.Fatalf("cannot open access log file: %s", err)
		}
	}
	return
}

func (c *Container) SSDPService(upnp upnp.Device) ssdp.Service {
	return ssdp.New(
		ssdp.Config{
			NotifyInterval: c.Config.NotifyInterval,
			Interfaces:     c.Config.ValidInterfaces,
			Server:         ServerToken,
			Location: func(ip net.IP) string {
				url, err := upnp.DDDLocation()
				if err != nil {
					panic(err)
				}
				return fmt.Sprintf("http://%s:%d%s", ip, c.Config.HTTP.Port, url)
			},
			UUID:     upnp.UniqueDeviceName(),
			Devices:  upnp.DeviceTypes(),
			Services: upnp.ServiceTypes(),
			BootID:   int32(time.Now().Unix() & 0x3fff), // TODO find the right mask
			ConfigID: upnp.ConfigID(),
		},
		c.logger("ssdp"),
	)
}

func (c *Container) UPNP(udn UDN, r *mux.Router, cdService *cds.Service) (dev upnp.Device, err error) {
	dev, err = upnp.NewDevice(
		upnp.DeviceSpec{
			DeviceType:       DeviceType,
			FriendlyName:     c.Config.FriendlyName,
			Manufacturer:     Manufacturer,
			ManufacturerURL:  ManufacturerURL,
			ModelDescription: ModelDescription,
			ModelName:        ModelName,
			ModelNumber:      ModelNumber,
			ModelURL:         ModelURL,
			UDN:              string(udn),
			UPC:              "000000",
			LastModified:     time.Unix(BuildDateUnixTS, 0),
		},
		r,
	)
	if err != nil {
		return
	}
	err = dev.AddService(cdService.Service)
	if err != nil {
		return
	}
	dev.AddIcon(upnp.Icon{"image/png", "/icons/md.png", 48, 48, 32})
	dev.AddIcon(upnp.Icon{"image/png", "/icons/lg.png", 128, 128, 32})
	return
}

type UDN string

func (c *Container) UDN() UDN {
	return UDN(fmt.Sprintf("uuid:%s", uuid.NewV5(uuid.NamespaceX500, c.Config.FriendlyName)))
}

func (c *Container) CDService(dir cds.ContentDirectory) *cds.Service {
	return cds.NewService(dir)
}

func (c *Container) FileServer(dir *cds.FilesystemContentDirectory) *cds.FileServer {
	return cds.NewFileServer(dir)
}

func (c *Container) ContentDirectory(dir *cds.ProcessingDirectory) cds.ContentDirectory {
	return cds.NewCache(dir, c.Config.Cache, c.logger("cd-cache"))
}

func (c *Container) ProcessingDirectory(
	dir *cds.FilesystemContentDirectory,
	fs *filesystem.Filesystem,
	fserver *cds.FileServer,
	ffprober *processor.FFProbeProcessor,
) (d *cds.ProcessingDirectory) {
	d = &cds.ProcessingDirectory{ContentDirectory: dir, Logger: c.logger("processing")}

	d.AddProcessor(100, fserver)
	d.AddProcessor(95, processor.NewAlbumArtProcessor(fs, c.logger("album-art")))
	d.AddProcessor(90, &processor.BasicIconProcessor{})

	if ffprober != nil {
		d.AddProcessor(80, ffprober)
	}

	return
}

func (c *Container) FFProbeProcessor() (p *processor.FFProbeProcessor) {
	l := c.logger("ffprobe")
	p, err := processor.NewFFProbeProcessor(c.Config.FFProbe, l)
	if err != nil {
		l.Errorf("cannot initialize ffprobe: %s", err.Error())
	}

	return
}

func (c *Container) FilesystemContentDirectory(fs *filesystem.Filesystem) *cds.FilesystemContentDirectory {
	return &cds.FilesystemContentDirectory{fs}
}

func (c *Container) Filesystem() (fs *filesystem.Filesystem, err error) {
	fs, err = filesystem.New(c.Config.Config)
	if err == nil {
		_, err = fs.Get(filesystem.RootID)
	}
	return
}

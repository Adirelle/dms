package main

//go:generate go-bindata data/

import (
	"bytes"
	"encoding/json"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"syscall"
	"time"

	"github.com/anacrolix/dms/ssdp"

	"github.com/anacrolix/dms/dlna/dms"
	"github.com/anacrolix/dms/ffmpeg"
	"github.com/anacrolix/dms/rrcache"
)

type dmsConfig struct {
	dms.Config
	Path             string
	IfName           string
	Http             string
	FFprobeCachePath string
	NoProbe          bool
}

func (c *dmsConfig) load(configPath string) {
	file, err := os.Open(configPath)
	if err != nil {
		log.Printf("config error (config file: '%s'): %v\n", configPath, err)
		return
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&c)
	if err != nil {
		log.Printf("config error: %v\n", err)
		return
	}
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
	ret = make([]net.Interface, len(ifaces))
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp != 0 && iface.MTU > 0 {
			ret = append(ret, iface)
		}
	}
	return
}

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)

	config := &dmsConfig{}
	configFilePath := ""

	flag.StringVar(&configFilePath, "config", "", "json configuration file")

	flag.StringVar(&config.Path, "path", ".", "browse root path")
	flag.StringVar(&config.Http, "http", ":1338", "http server port")
	flag.StringVar(&config.IfName, "ifname", "", "specific SSDP network interface")
	flag.StringVar(&config.FriendlyName, "friendlyName", "", "server friendly name")
	flag.StringVar(&config.FFprobeCachePath, "fFprobeCachePath", getDefaultFFprobeCachePath(), "path to FFprobe cache file")

	flag.DurationVar(&config.NotifyInterval, "notifyInterval", 30*time.Second, "interval between SSPD announces")

	flag.BoolVar(&config.LogHeaders, "logHeaders", false, "log HTTP headers")
	flag.BoolVar(&config.NoTranscode, "noTranscode", false, "disable transcoding")
	flag.BoolVar(&config.NoProbe, "noProbe", false, "disable media probing with ffprobe")
	flag.BoolVar(&config.StallEventSubscribe, "stallEventSubscribe", false, "workaround for some bad event subscribers")
	flag.BoolVar(&config.IgnoreHidden, "ignoreHidden", false, "ignore hidden files and directories")
	flag.BoolVar(&config.IgnoreUnreadable, "ignoreUnreadable", false, "ignore unreadable files and directories")

	flag.Parse()
	if flag.NArg() != 0 {
		flag.Usage()
		log.Fatalf("%s: %s\n", "unexpected positional arguments", flag.Args())
	}

	if len(configFilePath) > 0 {
		config.load(configFilePath)
	}

	var ffProber ffmpeg.FFProber
	if !config.NoProbe {
		cache := &fFprobeCache{
			c: rrcache.New(64 << 20),
		}
		if err := cache.load(config.FFprobeCachePath); err == nil {
			log.Print(err)
		}
		defer func() {
			if err := cache.save(config.FFprobeCachePath); err != nil {
				log.Print(err)
			}
		}()

		ffProber = ffmpeg.NewFFProber(config.NoProbe, cache)
	} else {
		ffProber = ffmpeg.NewFFProber(false, nil)
	}

	httpServer := startHTTPServer(config, ffProber)
	defer func() {
		if err := httpServer.Close(); err != nil {
			log.Fatal(err)
		}
	}()

	sspdConf := makeSSDPConfig(config, httpServer)
	adv := startAdvertiser(sspdConf)
	defer adv.Stop()
	resp := startResponder(sspdConf)
	defer resp.Stop()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
	<-sigs
}

func startHTTPServer(config *dmsConfig, ffprober *ffmpeg.FFProber) (httpServer *dms.Server) {
	httpServer = &dms.Server{
		Config: dmsConfig,
		Interfaces: func(ifName string) (ifs []net.Interface) {
			var err error
			if ifName == "" {
				ifs, err = net.Interfaces()
			} else {
				var if_ *net.Interface
				if_, err = net.InterfaceByName(ifName)
				if if_ != nil {
					ifs = append(ifs, *if_)
				}
			}
			if err != nil {
				log.Fatal(err)
			}
			var tmp []net.Interface
			for _, if_ := range ifs {
				if if_.Flags&net.FlagUp == 0 || if_.MTU <= 0 {
					continue
				}
				tmp = append(tmp, if_)
			}
			ifs = tmp
			return
		}(dmsConfig.IfName),
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
	}
	return
}

func makeSSDPConfig(config *dmsConfig, httpServer *dms.Server) *ssdp.SSDPConfig {
	return &ssdp.SSDPConfig{
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

func startAdvertiser(c *ssdp.SSDPConfig) *ssdp.Advertiser {
	a := ssdp.NewAdvertiser(*c)
	go a.Serve()
	return a
}

func startResponder(c *ssdp.SSDPConfig) *ssdp.Responder {
	r := ssdp.NewResponder(*c)
	go r.Serve()
	return r
}

func (cache *fFprobeCache) load(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	var items []dms.FfprobeCacheItem
	err = dec.Decode(&items)
	if err != nil {
		return err
	}
	for _, item := range items {
		cache.Set(item.Key, item.Value)
	}
	log.Printf("added %d items from cache", len(items))
	return nil
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

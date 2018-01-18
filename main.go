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

func (config *dmsConfig) load(configPath string) {
	file, err := os.Open(configPath)
	if err != nil {
		log.Printf("config error (config file: '%s'): %v\n", configPath, err)
		return
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&config)
	if err != nil {
		log.Printf("config error: %v\n", err)
		return
	}
}

//default config
var config = &dmsConfig{
	Http:             ":1338",
	FFprobeCachePath: getDefaultFFprobeCachePath(),
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

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)

	path := flag.String("path", config.Path, "browse root path")
	ifName := flag.String("ifname", config.IfName, "specific SSDP network interface")
	http := flag.String("http", config.Http, "http server port")
	friendlyName := flag.String("friendlyName", config.FriendlyName, "server friendly name")
	logHeaders := flag.Bool("logHeaders", config.LogHeaders, "log HTTP headers")
	fFprobeCachePath := flag.String("fFprobeCachePath", config.FFprobeCachePath, "path to FFprobe cache file")
	configFilePath := flag.String("config", "", "json configuration file")
	flag.BoolVar(&config.NoTranscode, "noTranscode", false, "disable transcoding")
	flag.BoolVar(&config.NoProbe, "noProbe", false, "disable media probing with ffprobe")
	flag.BoolVar(&config.StallEventSubscribe, "stallEventSubscribe", false, "workaround for some bad event subscribers")
	flag.DurationVar(&config.NotifyInterval, "notifyInterval", 30*time.Second, "interval between SSPD announces")
	flag.BoolVar(&config.IgnoreHidden, "ignoreHidden", false, "ignore hidden files and directories")
	flag.BoolVar(&config.IgnoreUnreadable, "ignoreUnreadable", false, "ignore unreadable files and directories")

	flag.Parse()
	if flag.NArg() != 0 {
		flag.Usage()
		log.Fatalf("%s: %s\n", "unexpected positional arguments", flag.Args())
	}

	config.Path = *path
	config.IfName = *ifName
	config.Http = *http
	config.FriendlyName = *friendlyName
	config.LogHeaders = *logHeaders
	config.FFprobeCachePath = *fFprobeCachePath

	if len(*configFilePath) > 0 {
		config.load(*configFilePath)
	}

	var ffProber ffmpeg.FFProber
	if !config.NoProbe {
		cache := &fFprobeCache{
			c: rrcache.New(64 << 20),
		}
		if err := cache.load(config.FFprobeCachePath); err == nil {
			log.Print(err)
		}

		ffProber = ffmpeg.NewFFProber(config.NoProbe, cache)
	} else {
		ffProber = ffmpeg.NewFFProber(false, nil)
	}

	dmsServer := &dms.Server{
		Config: config.Config,
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
		}(config.IfName),
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
	go func() {
		if err := dmsServer.Serve(); err != nil {
			log.Fatal(err)
		}
	}()
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)
	<-sigs
	err := dmsServer.Close()
	if err != nil {
		log.Fatal(err)
	}
	if err := cache.save(config.FFprobeCachePath); err != nil {
		log.Print(err)
	}
}

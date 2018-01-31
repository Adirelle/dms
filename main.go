package main

//go:generate go-bindata data/

import (
	"bytes"
	"flag"
	"log"
	"net"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"syscall"

	"github.com/anacrolix/dms/dlna/dms"
	"github.com/anacrolix/dms/ffmpeg"
	"github.com/anacrolix/dms/logging"
	"github.com/anacrolix/dms/rrcache"
	"github.com/anacrolix/dms/ssdp"
	"gopkg.in/thejerf/suture.v2"
)

func main() {
	config := &dmsConfig{Logging: logging.DefaultConfig()}

	setupFlags(config)

	flag.Parse()
	if flag.NArg() != 0 {
		flag.Usage()
		log.Printf("%s: %s\n", "unexpected positional arguments", flag.Args())
	}

	logger := logging.New(config.Logging)
	defer logger.Sync()

	ffProber, cache := makeFFProber(config, logger)
	if cache != nil {
		defer func() {
			if err := cache.save(config.FFprobeCachePath); err != nil {
				logger.Warnf("could not save cache: %s", err.Error())
			}
		}()
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

func makeFFProber(config *dmsConfig, logger logging.Logger) (ffmpeg.FFProber, *fFprobeCache) {
	if config.NoProbe {
		return ffmpeg.NewFFProber(true, nil), nil
	}

	cache := &fFprobeCache{
		c: rrcache.New(64 << 20),
	}
	if err := cache.load(config.FFprobeCachePath); err == nil {
		logger.Warnf("could load cache: %s", err.Error())
	}

	return ffmpeg.NewFFProber(false, cache), cache
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

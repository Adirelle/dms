package main

import (
	"bitbucket.org/anacrolix/dms/dlna"
	"bitbucket.org/anacrolix/dms/ffmpeg"
	"bitbucket.org/anacrolix/dms/misc"
	"bitbucket.org/anacrolix/dms/soap"
	"bitbucket.org/anacrolix/dms/ssdp"
	"bitbucket.org/anacrolix/dms/upnp"
	"bitbucket.org/anacrolix/dms/upnpav"
	"encoding/xml"
	"flag"
	"fmt"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"path"
	"strconv"
	"strings"
	"time"
)

const (
	serverField         = "Linux/3.4 DLNADOC/1.50 UPnP/1.0 DMS/1.0"
	rootDeviceType      = "urn:schemas-upnp-org:device:MediaServer:1"
	rootDeviceModelName = "dms 1.0"
	resPath             = "/res"
	rootDescPath        = "/rootDesc.xml"
	maxAge              = "30"
)

func makeDeviceUuid() string {
	/*
		buf := make([]byte, 16)
		if _, err := io.ReadFull(rand.Reader, buf); err != nil {
			panic(err)
		}
	*/
	var buf [16]byte
	return fmt.Sprintf("uuid:%x-%x-%x-%x-%x", buf[:4], buf[4:6], buf[6:8], buf[8:10], buf[10:])
}

type specVersion struct {
	Major int `xml:"major"`
	Minor int `xml:"minor"`
}

type icon struct {
	Mimetype, Width, Height, Depth, URL string
}

type service struct {
	XMLName     xml.Name `xml:"service"`
	ServiceType string   `xml:"serviceType"`
	ServiceId   string   `xml:"serviceId"`
	SCPDURL     string
	ControlURL  string `xml:"controlURL"`
	EventSubURL string `xml:"eventSubURL"`
}

type device struct {
	DeviceType   string `xml:"deviceType"`
	FriendlyName string `xml:"friendlyName"`
	Manufacturer string `xml:"manufacturer"`
	ModelName    string `xml:"modelName"`
	UDN          string
	IconList     []icon
	ServiceList  []service `xml:"serviceList>service"`
}

var services = []service{
	service{
		ServiceType: "urn:schemas-upnp-org:service:ContentDirectory:1",
		ServiceId:   "urn:upnp-org:serviceId:ContentDirectory",
		SCPDURL:     "/scpd/ContentDirectory.xml",
		ControlURL:  "/ctl/ContentDirectory",
	},
	/*
		service{
			ServiceType: "urn:schemas-upnp-org:service:ConnectionManager:1",
			ServiceId:   "urn:upnp-org:serviceId:ConnectionManager",
			SCPDURL:     "/scpd/ConnectionManager.xml",
			ControlURL:  "/ctl/ConnectionManager",
		},
	*/
}

type root struct {
	XMLName     xml.Name    `xml:"urn:schemas-upnp-org:device-1-0 root"`
	SpecVersion specVersion `xml:"specVersion"`
	Device      device      `xml:"device"`
}

func devices() []string {
	return []string{
		"urn:schemas-upnp-org:device:MediaServer:1",
	}
}

func serviceTypes() (ret []string) {
	for _, s := range services {
		ret = append(ret, s.ServiceType)
	}
	return
}
func httpPort() int {
	return httpConn.Addr().(*net.TCPAddr).Port
}

func serveHTTP() {
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Ext", "")
			w.Header().Set("Server", serverField)
			http.DefaultServeMux.ServeHTTP(w, r)
		}),
	}
	if err := srv.Serve(httpConn); err != nil {
		panic(err)
	}
	panic(nil)
}

func doSSDP() {
	active := map[string]bool{}
	for {
		ifs, err := net.Interfaces()
		if err != nil {
			panic(err)
		}
		for _, if_ := range ifs {
			if !active[if_.Name] {
				log.Println("starting SSDP server on", if_.Name)
				s := ssdp.Server{
					Interface: if_,
					Devices:   devices(),
					Services:  serviceTypes(),
					Location:  location,
					Server:    serverField,
					UUID:      rootDeviceUUID,
				}
				active[if_.Name] = true
				go s.Serve()
			}
		}
		time.Sleep(time.Second)
	}
}

var (
	rootDeviceUUID string
	httpConn       *net.TCPListener
	rootDescXML    []byte
	rootObjectPath string
)

func childCount(path_ string) int {
	f, err := os.Open(path_)
	if err != nil {
		log.Println(err)
		return 0
	}
	defer f.Close()
	fis, err := f.Readdir(-1)
	if err != nil {
		log.Println(err)
		return 0
	}
	ret := 0
	for _, fi := range fis {
		ret += len(fileEntries(fi, path_))
	}
	return ret
}

func itemResExtra(path string) (bitrate uint, duration string) {
	info, err := ffmpeg.Probe(path)
	if err != nil {
		log.Printf("error probing %s: %s", path, err)
		return
	}
	if _, err = fmt.Sscan(info.Format["bit_rate"], &bitrate); err != nil {
		panic(err)
	}
	if d := info.Format["duration"]; d != "N/A" {
		var f float64
		_, err = fmt.Sscan(info.Format["duration"], &f)
		if err != nil {
			log.Printf("probed duration for %s: %s\n", path, err)
		} else {
			duration = misc.FormatDurationSexagesimal(time.Duration(f * float64(time.Second)))
		}
	}
	return
}

func entryObject(parentID, host string, entry CDSEntry) interface{} {
	path_ := path.Join(entry.ParentPath, entry.FileInfo.Name())
	obj := upnpav.Object{
		ID:         path_,
		Title:      entry.Title,
		Restricted: 1,
		ParentID:   parentID,
	}
	if entry.FileInfo.IsDir() {
		obj.Class = "object.container.storageFolder"
		return upnpav.Container{
			Object:     obj,
			ChildCount: childCount(path_),
		}
	}
	mimeType := func() string {
		if entry.Transcode {
			return "video/mpeg"
		}
		return mime.TypeByExtension(path.Ext(entry.FileInfo.Name()))
	}()
	obj.Class = "object.item." + strings.SplitN(mimeType, "/", 2)[0] + "Item"
	values := url.Values{}
	values.Set("path", path_)
	if entry.Transcode {
		values.Set("transcode", "t")
	}
	url_ := &url.URL{
		Scheme:   "http",
		Host:     host,
		Path:     resPath,
		RawQuery: values.Encode(),
	}
	cf := dlna.ContentFeatures{}
	if entry.Transcode {
		cf.SupportTimeSeek = true
		cf.Transcoded = true
	} else {
		cf.SupportRange = true
	}
	mainRes := upnpav.Resource{
		ProtocolInfo: "http-get:*:" + mimeType + ":" + cf.String(),
		URL:          url_.String(),
		Size:         uint64(entry.FileInfo.Size()),
	}
	mainRes.Bitrate, mainRes.Duration = itemResExtra(path_)
	return upnpav.Item{
		Object: obj,
		Res:    []upnpav.Resource{mainRes},
	}
}

func fileEntries(fileInfo os.FileInfo, parentPath string) []CDSEntry {
	if fileInfo.IsDir() {
		return []CDSEntry{
			{fileInfo.Name(), fileInfo, parentPath, false},
		}
	}
	mimeType := mime.TypeByExtension(path.Ext(fileInfo.Name()))
	mimeTypeType := strings.SplitN(mimeType, "/", 2)[0]
	ret := []CDSEntry{
		{fileInfo.Name(), fileInfo, parentPath, false},
	}
	if mimeTypeType == "video" {
		ret = append(ret, CDSEntry{
			fileInfo.Name() + "/transcode",
			fileInfo,
			parentPath,
			true,
		})
	}
	return ret
}

type CDSEntry struct {
	Title      string
	FileInfo   os.FileInfo
	ParentPath string
	Transcode  bool
}

func ReadContainer(path_, parentID, host string) (ret []interface{}) {
	dir, err := os.Open(path_)
	if err != nil {
		log.Println(err)
		return
	}
	defer dir.Close()
	fis, err := dir.Readdir(-1)
	if err != nil {
		panic(err)
	}
	for _, fi := range fis {
		for _, entry := range fileEntries(fi, path_) {
			ret = append(ret, entryObject(parentID, host, entry))
		}
	}
	return
}

type Browse struct {
	ObjectID       string
	BrowseFlag     string
	Filter         string
	StartingIndex  int
	RequestedCount int
}

func contentDirectoryResponseArgs(sa upnp.SoapAction, argsXML []byte, host string) map[string]string {
	switch sa.Action {
	case "GetSortCapabilities":
		return map[string]string{
			"SortCaps": "dc:title",
		}
	case "Browse":
		var browse Browse
		if err := xml.Unmarshal([]byte(argsXML), &browse); err != nil {
			panic(err)
		}
		path := browse.ObjectID
		if path == "0" {
			path = rootObjectPath
		}
		switch browse.BrowseFlag {
		case "BrowseDirectChildren":
			objs := ReadContainer(path, browse.ObjectID, host)
			totalMatches := len(objs)
			objs = objs[browse.StartingIndex:]
			if browse.RequestedCount != 0 && int(browse.RequestedCount) < len(objs) {
				objs = objs[:browse.RequestedCount]
			}
			result, err := xml.MarshalIndent(objs, "", "  ")
			if err != nil {
				panic(err)
			}
			return map[string]string{
				"TotalMatches":   fmt.Sprint(totalMatches),
				"NumberReturned": fmt.Sprint(len(objs)),
				"Result":         didl_lite(string(result)),
			}
		default:
			log.Println("unhandled browse flag:", browse.BrowseFlag)
		}
	default:
		log.Println("unhandled content directory action:", sa.Action)
	}
	return nil
}

func serveDLNATranscode(w http.ResponseWriter, r *http.Request, path_ string) {
	w.Header().Set(dlna.TransferModeDomain, "Streaming")
	dlnaRange := r.Header.Get(dlna.TimeSeekRangeDomain)
	var start, end string
	if dlnaRange != "" {
		if !strings.HasPrefix(dlnaRange, "npt=") {
			log.Println("bad range:", dlnaRange)
			return
		}
		range_, err := dlna.ParseNPTRange(dlnaRange[len("npt="):])
		if err != nil {
			log.Println("bad range:", dlnaRange)
			return
		}
		start = strconv.FormatFloat(float64(range_.Start)/float64(time.Second), 'f', -1, 64)
		if range_.End >= 0 {
			end = strconv.FormatFloat(float64(range_.End)/float64(time.Second), 'f', -1, 64)
		}
		w.Header().Set(dlna.TimeSeekRangeDomain, range_.String())
	}
	cmd := exec.Command("/media/data/pydlnadms/transcode", path_, start, end)
	cmd.Stdout = w
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		log.Println(err)
		return
	}
	w.WriteHeader(206)
	err := cmd.Wait()
	if err != nil {
		log.Println(err)
	}
}

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)
	rootDeviceUUID = makeDeviceUuid()
	flag.StringVar(&rootObjectPath, "path", ".", "browse root path")
	flag.Parse()
	var err error
	rootDescXML, err = xml.MarshalIndent(
		root{
			SpecVersion: specVersion{Major: 1, Minor: 0},
			Device: device{
				DeviceType: rootDeviceType,
				FriendlyName: fmt.Sprintf("%s: %s on %s", rootDeviceModelName, func() string {
					user, err := user.Current()
					if err != nil {
						panic(err)
					}
					return user.Name
				}(),
					func() string {
						name, err := os.Hostname()
						if err != nil {
							panic(err)
						}
						return name
					}()),
				Manufacturer: "Matt Joiner <anacrolix@gmail.com>",
				ModelName:    rootDeviceModelName,
				UDN:          rootDeviceUUID,
				ServiceList:  services,
			},
		},
		" ", "  ")
	if err != nil {
		panic(err)
	}
	rootDescXML = append([]byte(`<?xml version="1.0"?>`), rootDescXML...)
	log.Println(string(rootDescXML))
	if err != nil {
		panic(err)
	}
	httpConn, err = net.ListenTCP("tcp", &net.TCPAddr{Port: 1338})
	if err != nil {
		panic(err)
	}
	defer httpConn.Close()
	log.Println("HTTP server on", httpConn.Addr())
	http.HandleFunc("/", func(http.ResponseWriter, *http.Request) {
		panic(nil)
	})
	http.HandleFunc(resPath, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			log.Println(err)
		}
		path := r.Form.Get("path")
		if r.Form.Get("transcode") == "" {
			log.Printf("serving file %s: %s\n", r.Header.Get("Range"), path)
			http.ServeFile(w, r, path)
			return
		}
		serveDLNATranscode(w, r, path)
	})
	http.HandleFunc(rootDescPath, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", `text/xml; charset="utf-8"`)
		w.Header().Set("content-length", fmt.Sprint(len(rootDescXML)))
		w.Header().Set("server", serverField)
		w.Write(rootDescXML)
	})
	http.HandleFunc("/ctl/ContentDirectory", func(w http.ResponseWriter, r *http.Request) {
		soapActionString := r.Header.Get("SOAPACTION")
		soapAction, ok := upnp.ParseActionHTTPHeader(soapActionString)
		if !ok {
			log.Println("invalid soapaction:", soapActionString)
			return
		}
		log.Printf("SOAP request from %s: %s\n", r.RemoteAddr, soapAction)
		var env soap.Envelope
		if err := xml.NewDecoder(r.Body).Decode(&env); err != nil {
			panic(err)
		}
		w.Header().Set("Content-Type", `text/xml; charset="utf-8"`)
		w.Header().Set("Ext", "")
		w.Header().Set("Server", serverField)
		actionResponseXML, err := xml.MarshalIndent(func() soap.Action {
			argMap := contentDirectoryResponseArgs(soapAction, env.Body.Action, r.Host)
			args := make([]soap.Arg, 0, len(argMap))
			for argName, value := range argMap {
				args = append(args, soap.Arg{
					xml.Name{Local: argName},
					value,
				})
			}
			return soap.Action{
				xml.Name{
					Space: soapAction.ServiceURN.String(),
					Local: soapAction.Action + "Response",
				},
				args,
			}
		}(), "", "  ")
		if err != nil {
			panic(err)
		}
		body, err := xml.MarshalIndent(soap.NewEnvelope(actionResponseXML), "", "  ")
		if err != nil {
			panic(err)
		}
		body = append([]byte(xml.Header), body...)
		if _, err := w.Write(body); err != nil {
			panic(err)
		}
	})
	go serveHTTP()
	doSSDP()
}

func didl_lite(chardata string) string {
	return `<DIDL-Lite` +
		` xmlns:dc="http://purl.org/dc/elements/1.1/"` +
		` xmlns:upnp="urn:schemas-upnp-org:metadata-1-0/upnp/"` +
		` xmlns="urn:schemas-upnp-org:metadata-1-0/DIDL-Lite/"` +
		` xmlns:dlna="urn:schemas-dlna-org:metadata-1-0/">` +
		chardata +
		`</DIDL-Lite>`
}

func location(ip net.IP) string {
	url := url.URL{
		Scheme: "http",
		Host: (&net.TCPAddr{
			IP:   ip,
			Port: httpPort(),
		}).String(),
		Path: rootDescPath,
	}
	return url.String()
}

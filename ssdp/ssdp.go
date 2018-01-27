package ssdp

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/textproto"
	"strings"
	"time"
)

const (
	AddrString = "239.255.255.250:1900"
	rootDevice = "upnp:rootdevice"
	aliveNTS   = "ssdp:alive"
	byebyeNTS  = "ssdp:byebye"
)

var NetAddr = &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 1900}

type badStringError struct {
	what string
	str  string
}

func (e *badStringError) Error() string { return fmt.Sprintf("%s %q", e.what, e.str) }

func ReadRequest(b *bufio.Reader) (req *http.Request, err error) {
	tp := textproto.NewReader(b)
	var s string
	if s, err = tp.ReadLine(); err != nil {
		return nil, err
	}
	defer func() {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
	}()

	var f []string
	// TODO a split that only allows N values?
	if f = strings.SplitN(s, " ", 3); len(f) < 3 {
		return nil, &badStringError{"malformed request line", s}
	}
	if f[1] != "*" {
		return nil, &badStringError{"bad URL request", f[1]}
	}
	req = &http.Request{
		Method: f[0],
	}
	var ok bool
	if req.ProtoMajor, req.ProtoMinor, ok = http.ParseHTTPVersion(strings.TrimSpace(f[2])); !ok {
		return nil, &badStringError{"malformed HTTP version", f[2]}
	}

	mimeHeader, err := tp.ReadMIMEHeader()
	if err != nil {
		return nil, err
	}
	req.Header = http.Header(mimeHeader)
	return
}

type SSDPConfig struct {
	Interfaces     func() ([]net.Interface, error)
	Server         string
	Services       []string
	Devices        []string
	Location       func(net.IP) string
	UUID           string
	NotifyInterval time.Duration
	BootID         int32
	ConfigID       int32
}

func (c *SSDPConfig) usnFromTarget(target string) string {
	if target == c.UUID {
		return target
	}
	return c.UUID + "::" + target
}

func (c *SSDPConfig) allTypes() []string {
	return append(
		append([]string{rootDevice, c.UUID}, c.Devices...),
		c.Services...,
	)
}

// type Server struct {
// 	SSDPConfig
// 	conn   *net.UDPConn
// 	closed chan struct{}
// }

// func makeConn(ifi net.Interface) (ret *net.UDPConn, err error) {
// 	ret, err = net.ListenMulticastUDP("udp", &ifi, NetAddr)
// 	if err != nil {
// 		return
// 	}
// 	p := ipv4.NewPacketConn(ret)
// 	if err := p.SetMulticastTTL(2); err != nil {
// 		log.Println(err)
// 	}
// 	if err := p.SetMulticastLoopback(true); err != nil {
// 		log.Println(err)
// 	}
// 	return
// }

// func (me *Server) serve() {
// 	for {
// 		b := make([]byte, me.Interface.MTU)
// 		n, addr, err := me.conn.ReadFromUDP(b)
// 		select {
// 		case <-me.closed:
// 			return
// 		default:
// 		}
// 		if err != nil {
// 			me.logf("error reading from UDP socket: %s", err)
// 			break
// 		}
// 		go me.handle(b[:n], addr)
// 	}
// }

// func (me *Server) Init() (err error) {
// 	me.closed = make(chan struct{})
// 	me.conn, err = makeConn(me.Interface)
// 	return
// }

// func (me *Server) Close() {
// 	close(me.closed)
// 	me.sendByeBye()
// 	me.conn.Close()
// }

// func (me *Server) Serve() (err error) {
// 	go me.serve()
// 	for {
// 		me.notifyAll(aliveNTS)
// 		time.Sleep(me.NotifyInterval)
// 	}
// }

// func (me *Server) multicastSourceAddrs() (ips []net.IP, err error) {
// 	ifAddrs, err := me.Interface.Addrs()
// 	if err != nil {
// 		return
// 	}
// 	ips = make([]net.IP, len(ifAddrs))
// 	for _, addr := range ifAddrs {
// 		var ip net.IP
// 		switch val := addr.(type) {
// 		case *net.IPNet:
// 			ip = val.IP
// 		case *net.IPAddr:
// 			ip = val.IP
// 		default:
// 			// me.logf("ignoring unknown address type %s", addr.String())
// 			continue
// 		}
// 		if ip.To4() == nil || ip.String() == "" {
// 			// me.logf("ignoring non-IPv4 address %s", addr.String())
// 			continue
// 		}
// 		ips = append(ips, ip)
// 	}
// 	if len(ips) == 0 {
// 		err = fmt.Errorf("no source address")
// 	}
// 	return
// }

// func (me *Server) usnFromTarget(target string) string {
// 	if target == me.UUID {
// 		return target
// 	}
// 	return me.UUID + "::" + target
// }

// func (me *Server) makeNotifyMessage(target, nts string, ip net.IP) []byte {

// 	h := me.defaultHeaders()
// 	h["HOST"] = []string{AddrString}
// 	h["NT"] = []string{target}
// 	h["NTS"] = []string{nts}
// 	h["USN"] = []string{me.usnFromTarget(target)}
// 	h["LOCATION"] = []string{me.Location(ip)}

// 	req := http.Request{
// 		Method:     "NOTIFY",
// 		URL:        &url.URL{Path: "*"},
// 		Proto:      "HTTP/1.1",
// 		ProtoMajor: 1,
// 		ProtoMinor: 1,
// 		Header:     http.Header(h),
// 	}

// 	buf := &bytes.Buffer{}
// 	req.Write(buf)
// 	return buf.Bytes()
// }

// func (me *Server) send(buf []byte, addr *net.UDPAddr) {
// 	me.sendOnce(buf, addr)
// 	time.Sleep(10 * time.Millisecond)
// 	me.sendOnce(buf, addr)
// }

// func (me *Server) sendOnce(buf []byte, addr *net.UDPAddr) {
// 	if n, err := me.conn.WriteToUDP(buf, addr); err != nil {
// 		me.logf("error writing to UDP socket: %s", err)
// 	} else if n != len(buf) {
// 		me.logf("short write: %d/%d bytes", n, len(buf))
// 	}
// }

// func (me *Server) delayedSend(delay time.Duration, buf []byte, addr *net.UDPAddr) {
// 	go func() {
// 		select {
// 		case <-time.After(delay):
// 			me.send(buf, addr)
// 		case <-me.closed:
// 		}
// 	}()
// }

// func (me *Server) log(args ...interface{}) {
// 	args = append([]interface{}{me.Interface.Name + ": "}, args...)
// 	log.Print(args...)
// }

// func (me *Server) logf(f string, args ...interface{}) {
// 	me.log(fmt.Sprintf(f, args...))
// }

// func (me *Server) sendByeBye() {
// 	for _, type_ := range me.allTypes() {
// 		buf := me.makeNotifyMessage(type_, byebyeNTS, nil)
// 		me.send(buf, NetAddr)
// 	}
// }

// func (me *Server) notifyAll(nts string) (err error) {
// 	ips, err := me.multicastSourceAddrs()
// 	if err != nil {
// 		return
// 	}
// 	all := sync.WaitGroup{}
// 	for _, ip := range ips {
// 		conn, err := net.DialUDP("udp", &net.UDPAddr{IP: ip}, NetAddr)
// 		if err != nil {
// 			me.logf("cannot multicast from %s: %s", ip.String(), err.Error())
// 			continue
// 		}
// 		sent := sync.WaitGroup{}
// 		for _, msgType := range me.allTypes() {
// 			buf := me.makeNotifyMessage(msgType, nts, ip)
// 			delay := time.Duration(rand.Int63n(int64(100 * time.Millisecond)))
// 			time.AfterFunc(delay, func() {
// 				defer sent.Done()
// 				defer all.Done()
// 				for i := 0; i < 2; i++ {
// 					n, err := conn.Write(buf)
// 					if err != nil {
// 						me.logf("could not send notify from %s: %s", ip.String(), err.Error())
// 					} else if n < len(buf) {
// 						me.logf("short write %d/%d", n, len(buf))
// 					} else {
// 						me.logf("%d sent to %s from %s", n, conn.RemoteAddr().String(), conn.LocalAddr().String())
// 					}
// 					time.Sleep(10 * time.Millisecond)
// 				}
// 			})
// 			sent.Add(1)
// 			all.Add(1)
// 		}
// 		go func() {
// 			sent.Wait()
// 			conn.Close()
// 		}()
// 	}
// 	all.Wait()
// 	return
// }

// func (me *Server) allTypes() (ret []string) {
// 	for _, a := range [][]string{
// 		{rootDevice, me.UUID},
// 		me.Devices,
// 		me.Services,
// 	} {
// 		ret = append(ret, a...)
// 	}
// 	return
// }

// func (me *Server) handle(buf []byte, sender *net.UDPAddr) {
// 	req, err := ReadRequest(bufio.NewReader(bytes.NewReader(buf)))
// 	if err != nil {
// 		me.log(err)
// 		return
// 	}
// 	if req.Method != "M-SEARCH" || req.Header.Get("man") != `"ssdp:discover"` {
// 		return
// 	}
// 	var mx uint
// 	if req.Header.Get("Host") == AddrString {
// 		mxHeader := req.Header.Get("mx")
// 		i, err := strconv.ParseUint(mxHeader, 0, 0)
// 		if err != nil {
// 			me.logf("Invalid mx header %q: %s", mxHeader, err)
// 			return
// 		}
// 		mx = uint(i)
// 	} else {
// 		mx = 1
// 	}
// 	types := func(st string) []string {
// 		if st == "ssdp:all" {
// 			return me.allTypes()
// 		}
// 		for _, t := range me.allTypes() {
// 			if t == st {
// 				return []string{t}
// 			}
// 		}
// 		return nil
// 	}(req.Header.Get("st"))
// 	for _, ip := range func() (ret []net.IP) {
// 		addrs, err := me.Interface.Addrs()
// 		if err != nil {
// 			panic(err)
// 		}
// 		for _, addr := range addrs {
// 			if ip, ok := func() (net.IP, bool) {
// 				switch data := addr.(type) {
// 				case *net.IPNet:
// 					if data.Contains(sender.IP) {
// 						return data.IP, true
// 					}
// 					return nil, false
// 				case *net.IPAddr:
// 					return data.IP, true
// 				}
// 				panic(addr)
// 			}(); ok {
// 				ret = append(ret, ip)
// 			}
// 		}
// 		return
// 	}() {
// 		for _, type_ := range types {
// 			resp := me.makeResponse(ip, type_, req)
// 			delay := time.Duration(rand.Int63n(int64(time.Second) * int64(mx)))
// 			me.delayedSend(delay, resp, sender)
// 		}
// 	}
// }

// func (me *Server) makeResponse(ip net.IP, targ string, req *http.Request) (ret []byte) {
// 	h := me.defaultHeaders()
// 	h["LOCATION"] = []string{me.Location(ip)}
// 	h["ST"] = []string{targ}
// 	h["USN"] = []string{me.usnFromTarget(targ)}
// 	resp := &http.Response{
// 		StatusCode: 200,
// 		ProtoMajor: 1,
// 		ProtoMinor: 1,
// 		Header:     http.Header(h),
// 		Request:    req,
// 	}
// 	buf := &bytes.Buffer{}
// 	if err := resp.Write(buf); err != nil {
// 		panic(err)
// 	}
// 	return buf.Bytes()
// }

// func (me *Server) defaultHeaders() map[string][]string {
// 	return map[string][]string{
// 		"CACHE-CONTROL":     {fmt.Sprintf("max-age=%d", 5*me.NotifyInterval/2/time.Second)},
// 		"EXT":               {""},
// 		"SERVER":            {me.Server},
// 		"BOOTID.UPNP.ORG":   {me.BootID},
// 		"CONFIGID.UPNP.ORG": {me.ConfigID},
// 		"DATE":              {time.Now().Format(time.RFC1123)},
// 	}
// }

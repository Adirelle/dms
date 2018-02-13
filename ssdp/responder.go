package ssdp

import (
	"bufio"
	"bytes"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/anacrolix/dms/logging"
	"golang.org/x/net/ipv4"
)

type Responder struct {
	Config
	conn *ipv4.PacketConn
	logging.Logger
	done chan struct{}
	sync.WaitGroup
}

func NewResponder(c Config, l logging.Logger) *Responder {
	return &Responder{Config: c, Logger: l}
}

func (r *Responder) String() string {
	return "ssdp.Responder"
}

func (r *Responder) Port() int {
	return r.conn.LocalAddr().(*net.UDPAddr).Port
}

func (r *Responder) Serve() {
	r.done = make(chan struct{})
	r.Add(1)
	defer func() {
		r.conn.Close()
		r.Done()
	}()
	r.Info("responder starting")

	var err error

	r.conn, err = r.makeConn()
	if err != nil {
		r.Errorf(err.Error())
		return
	}

	r.Infof("listening for SSDP requests on %s", r.conn.LocalAddr().String())
	for {
		sender, req, err := r.receiveRequest()
		select {
		case <-r.done:
			return
		default:
		}
		if err == nil {
			go r.handle(sender, req)
		} else if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
			r.Infof("error while receiving: %s", err.Error())
		} else {
			r.Warnf("error while receiving: %s", err.Error())
		}
	}
}

func (r *Responder) Stop() {
	close(r.done)
	r.conn.Close()
	r.Wait()
	r.Info("responder stopped")
}

func (r *Responder) makeConn() (conn *ipv4.PacketConn, err error) {
	ifaces, err := r.Interfaces()
	if err != nil {
		return
	}
	c, err := net.ListenUDP("udp4", NetAddr)
	if err != nil {
		return
	}
	conn = ipv4.NewPacketConn(c)
	for _, iface := range ifaces {
		if iErr := conn.JoinGroup(&iface, NetAddr); iErr != nil {
			r.Warnf("could not join multicast group on %q: %s", iface.Name, iErr.Error())
		}
	}
	return
}

func (r *Responder) receiveRequest() (sender *net.UDPAddr, req *http.Request, err error) {
	var buf [2048]byte
	n, _, sdr, err := r.conn.ReadFrom(buf[:])
	if err != nil {
		return
	}
	sender, _ = sdr.(*net.UDPAddr)
	req, err = http.ReadRequest(bufio.NewReader(bytes.NewReader(buf[:n])))
	return
}

func (r *Responder) handle(sender *net.UDPAddr, req *http.Request) {
	log := r.Logger.With(
		zap.Namespace("request"),
		"remote", sender.String(),
		"method", req.Method,
		"url", req.URL.String(),
	)
	if req.Method != "M-SEARCH" || req.URL.String() != "*" || req.Header.Get("MAN") != `"ssdp:discover"` {
		log.Debugw("ignored request")
		return
	}

	conn, err := r.openReplyConn(sender, req.Header.Get("TCPPORT.UPNP.ORG"), log)
	if err != nil {
		log.Debugf("could not open reply connection: %s", err.Error())
		return
	}
	log = log.With(
		zap.Namespace("response"),
		"local", conn.LocalAddr().String(),
		"remote", conn.RemoteAddr().String(),
		"net", conn.LocalAddr().Network(),
	)

	maxDelay := readMaxDelay(req.Header, log)
	sts := r.resolveST(req.Header.Get("ST"))
	if len(sts) == 0 {
		log.Debugf("no notification types matching %q", req.Header.Get("ST"))
		return
	}
	maxDelay = maxDelay / time.Duration(len(sts))

	for _, st := range sts {
		r.sendResponse(conn, st, maxDelay, log.With("st", st))
	}
}

func (r *Responder) openReplyConn(sender *net.UDPAddr, tcpPortHeader string, log logging.Logger) (conn net.Conn, err error) {
	ip, err := r.findLocalIPFor(sender)
	if err != nil {
		return
	}

	if tcpPortHeader == "" {
		return net.DialUDP("udp", &net.UDPAddr{IP: ip}, sender)
	}

	port, err := strconv.Atoi(tcpPortHeader)
	if err != nil {
		return
	}
	return net.DialTCP("tcp", &net.TCPAddr{IP: ip}, &net.TCPAddr{IP: sender.IP, Port: port})
}

func (r *Responder) sendResponse(conn net.Conn, st string, maxDelay time.Duration, log logging.Logger) {
	delay := time.Duration(rand.Int63n(int64(maxDelay)))
	select {
	case <-time.After(delay):
	case <-r.done:
		return
	}
	_, err := r.writeResponse(conn, mustGetIP(conn.LocalAddr()), st)
	if err != nil {
		log.Warnf("could not send: %s", err.Error())
	} else {
		log.Debugf("response sent")
	}
}

func readMaxDelay(headers http.Header, log logging.Logger) time.Duration {
	mx := headers.Get("MX")
	if headers.Get("TCPPORT.UPNP.ORG") != "" || mx == "" {
		return time.Second
	}
	n, err := strconv.Atoi(mx)
	if err != nil {
		log.Debugf("invalid mx (%q): %s", mx, err.Error())
		return time.Second
	}
	if n < 0 {
		n = 1
	} else if n > 5 {
		n = 5
	}
	return time.Duration(n) * time.Second
}

func (r *Responder) resolveST(st string) []string {
	types := r.allTypes()
	if st == "ssdp:all" {
		return types
	}
	for _, t := range types {
		if t == st {
			return []string{st}
		}
	}
	return nil
}

const responseTpl = "HTTP/1.1 200 OK\r\n" +
	"CACHE-CONTROL: max-age=%d\r\n" +
	"DATE: %s\r\n" +
	"EXT:\r\n" +
	"LOCATION: %s\r\n" +
	"SERVER: %s\r\n" +
	"ST: %s\r\n" +
	"USN: %s\r\n" +
	"BOOTID.UPNP.ORG: %d\r\n" +
	"CONFIGID.UPNP.ORG: %d\r\n" +
	"\r\n"

func (r *Responder) writeResponse(conn net.Conn, ip net.IP, st string) (int, error) {
	return fmt.Fprintf(
		conn,
		responseTpl,
		5*r.NotifyInterval/2/time.Second,
		time.Now().Format(time.RFC1123),
		r.Location(ip),
		r.Server,
		st,
		r.usnFromTarget(st),
		r.BootID,
		r.ConfigID,
	)
}

func (r *Responder) findLocalIPFor(sender *net.UDPAddr) (found net.IP, err error) {
	ifaces, err := r.Interfaces()
	if err != nil {
		return
	}
	senderIP := sender.IP
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			switch val := addr.(type) {
			case *net.IPNet:
				if val.Contains(senderIP) || val.IP.Equal(senderIP) {
					return val.IP, nil
				}
			case *net.IPAddr:
				if val.IP.Equal(senderIP) {
					return val.IP, nil
				}
			default:
				r.Debugf("ignoring unhandled addr type %#v", addr)
			}
		}
	}
	return nil, fmt.Errorf("no local addr found for %s", senderIP.String())
}

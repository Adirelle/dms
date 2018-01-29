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

	"github.com/anacrolix/dms/logging"
	"github.com/thejerf/suture"
	"golang.org/x/net/ipv4"
)

type Responder struct {
	SSDPConfig
	uni   *ipv4.PacketConn
	multi *ipv4.PacketConn
	l     logging.Logger
	*suture.Supervisor
}

func NewResponder(c SSDPConfig, l logging.Logger) *Responder {
	return &Responder{SSDPConfig: c, l: l.Named("responder")}
}

func (r *Responder) String() string {
	return "ssdp.responder"
}

func (r *Responder) Port() int {
	return r.uni.LocalAddr().(*net.UDPAddr).Port
}

func (r *Responder) Serve() {
	var err error
	r.multi, err = r.makeMulticastConn()
	if err != nil {
		r.l.Errorf("could not bind multicast listener: %s", err.Error())
		return
	}

	for port := 1900; port < 0xFFFF; port++ {
		r.uni, err = r.makeUnicastConn(port)
		if err == nil {
			r.l.Errorf("listening for unicast requests on port %d", port)
			break
		} else {
			r.l.Errorf("could not bind unicast listener on port %d: %s", port, err.Error())
		}
	}

	r.Supervisor = suture.NewSimple("ssdp.responder")
	r.Add(newListener(r.SSDPConfig, r.uni, r.l.Named("unicast")))
	r.Add(newListener(r.SSDPConfig, r.multi, r.l.Named("multicast")))
	r.Supervisor.Serve()
}

func (r *Responder) makeMulticastConn() (conn *ipv4.PacketConn, err error) {
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
		if iErr := conn.JoinGroup(&iface, NetAddr); iErr == nil {
			r.l.Infof("listening on %q", iface.Name)
		} else {
			r.l.Errorf("could not join multicast group on %q: %s", iface.Name, iErr.Error())
		}
	}
	return
}

func (r *Responder) makeUnicastConn(port int) (conn *ipv4.PacketConn, err error) {
	c, err := net.ListenUDP("udp4", &net.UDPAddr{Port: port})
	if err == nil {
		conn = ipv4.NewPacketConn(c)
	}
	return
}

type listener struct {
	SSDPConfig
	conn *ipv4.PacketConn
	done chan struct{}
	sync.WaitGroup
	logging.Logger
}

func newListener(c SSDPConfig, conn *ipv4.PacketConn, l logging.Logger) *listener {
	return &listener{
		SSDPConfig: c,
		Logger:     l.With("socket", conn.LocalAddr().String()),
		conn:       conn,
	}
}

func (l *listener) String() string {
	return fmt.Sprintf("ssdp.listener.%s", l.conn.LocalAddr().String())
}

func (l *listener) Serve() {
	l.done = make(chan struct{})
	l.Add(1)
	defer func() {
		l.conn.Close()
		l.Done()
	}()
	for {
		msg := make([]byte, 2048)
		n, _, sender, err := l.conn.ReadFrom(msg)
		select {
		case <-l.done:
			return
		default:
		}
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
				l.Infof("error while receiving: %s", err.Error())
			} else {
				l.Errorf("error while receiving: %s", err.Error())
			}
			continue
		}
		go l.handle(sender.(*net.UDPAddr), msg[:n], l.With("client", sender.String()))
	}
}

func (l *listener) Stop() {
	close(l.done)
	l.conn.Close()
	l.Wait()
}

func (l *listener) handle(sender *net.UDPAddr, msg []byte, log logging.Logger) {
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(msg)))
	if err != nil {
		log.Errorf("cannot read requests: %s", err.Error())
		return
	}

	log = log.With(
		"method", req.Method,
		"url", req.URL.String(),
		"headers", req.Header,
	)
	if req.Method != "M-SEARCH" || req.URL.String() != "*" || req.Header.Get("Man") != `"ssdp:discover"` {
		log.Debugw("ignored request")
		return
	}

	if tcpPort := req.Header.Get("TCPPORT.UPNP.ORG"); tcpPort != "" {
		log.Warnf("ignored M-SEARCH, cannot reply to TCP port %s", tcpPort)
		return
	}

	ip, err := l.findLocalIPFor(sender)
	if err != nil {
		log.Errorf("could not find a local addr to reply: %s", err.Error())
		return
	}
	log = log.With("localAddr", ip.String())

	maxDelay := readMaxDelay(req.Header.Get("Mx"), l)
	sts := l.resolveST(req.Header.Get("St"))
	if len(sts) == 0 {
		log.Debugf("no matching notification types", req.Header.Get("St"))
		return
	}
	maxDelay = maxDelay / time.Duration(len(sts))

	for _, st := range sts {
		msg := l.makeResponse(ip, st)
		delay := time.Duration(rand.Int63n(int64(maxDelay)))
		select {
		case <-time.After(delay):
		case <-l.done:
			return
		}
		if n, err := l.conn.WriteTo(msg, nil, sender); err != nil {
			log.Errorf("could not send: %s", err.Error())
		} else if n < len(msg) {
			log.Errorf("short write: %d/%d", n, len(msg))
		} else {
			log.Debugf("response sent")
		}
	}
}

func readMaxDelay(mx string, l logging.Logger) time.Duration {
	if mx == "" {
		return time.Second
	}
	n, err := strconv.Atoi(mx)
	if err != nil {
		l.Debugf("invalid mx (%q): %s", mx, err.Error())
		return time.Second
	}
	if n < 0 {
		n = 1
	} else if n > 5 {
		n = 5
	}
	return time.Duration(n) * time.Second
}

func (l *listener) resolveST(st string) []string {
	types := l.allTypes()
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

func (l *listener) makeResponse(ip net.IP, st string) []byte {
	s := fmt.Sprintf(
		responseTpl,
		5*l.NotifyInterval/2/time.Second,
		time.Now().Format(time.RFC1123),
		l.Location(ip),
		l.Server,
		st,
		l.usnFromTarget(st),
		l.BootID,
		l.ConfigID,
	)
	return []byte(s)
}

func (l *listener) findLocalIPFor(sender *net.UDPAddr) (found net.IP, err error) {
	ifaces, err := l.Interfaces()
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
				if val.Contains(senderIP) {
					return val.IP, nil
				}
			case *net.IPAddr:
				if val.IP.Equal(senderIP) {
					return val.IP, nil
				}
			}
		}
	}
	return nil, fmt.Errorf("no local addr found for %s", senderIP.String())
}

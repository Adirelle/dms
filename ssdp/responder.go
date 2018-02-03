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
	"gopkg.in/thejerf/suture.v2"
)

type Responder struct {
	Config
	uni   *ipv4.PacketConn
	multi *ipv4.PacketConn
	l     logging.Logger
	*suture.Supervisor
}

func NewResponder(c Config, l logging.Logger) *Responder {
	return &Responder{Config: c, l: l.Named("responder")}
}

func (r *Responder) String() string {
	return "ssdp.responder"
}

func (r *Responder) Port() int {
	return r.uni.LocalAddr().(*net.UDPAddr).Port
}

func (r *Responder) Serve() {
	var err error
	for port := 1900; port < 0xFFFF; port++ {
		r.uni, err = r.makeUnicastConn(port)
		if err == nil {
			r.l.Infof("listening for unicast requests on %s", r.uni.LocalAddr().String())
			break
		} else {
			r.l.Warn(err)
		}
	}

	r.multi, err = r.makeMulticastConn()
	if err != nil {
		r.l.Error(err)
		return
	}

	r.Supervisor = suture.NewSimple("ssdp.responder")
	r.Add(newListener(r.Config, r.uni, r.l.Named("unicast")))
	r.Add(newListener(r.Config, r.multi, r.l.Named("multicast")))
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
			r.l.Infof("listening for multicast requests on %q (%s)", iface.Name, conn.LocalAddr().String())
		} else {
			r.l.Warnf("could not join multicast group on %q: %s", iface.Name, iErr.Error())
		}
	}
	return
}

func (r *Responder) makeUnicastConn(port int) (conn *ipv4.PacketConn, err error) {
	ifaces, err := r.Interfaces()
	if err != nil {
		return
	}
	var addr = &net.UDPAddr{Port: port}
	if len(ifaces) == 1 {
		addrs, err := ifaces[0].Addrs()
		if err != nil {
			return nil, err
		}
		for _, a := range addrs {
			if ip, ok := getIP(a); ok && ip.To4() != nil {
				addr.IP = ip
				break
			}
		}
	}

	c, err := net.ListenUDP("udp4", addr)
	if err == nil {
		conn = ipv4.NewPacketConn(c)
	}
	return
}

type listener struct {
	Config
	conn *ipv4.PacketConn
	done chan struct{}
	sync.WaitGroup
	logging.Logger
}

func newListener(c Config, conn *ipv4.PacketConn, l logging.Logger) *listener {
	return &listener{
		Config: c,
		Logger: l.With("local", conn.LocalAddr().String()),
		conn:   conn,
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
		sender, req, err := l.receiveRequest()
		select {
		case <-l.done:
			return
		default:
		}
		if err == nil {
			go l.handle(sender, req, l)
		} else if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
			l.Infof("error while receiving: %s", err.Error())
		} else {
			l.Warnf("error while receiving: %s", err.Error())
		}
	}
}

func (l *listener) Stop() {
	close(l.done)
	l.conn.Close()
	l.Wait()
}

func (l *listener) receiveRequest() (sender *net.UDPAddr, req *http.Request, err error) {
	var buf [2048]byte
	n, _, sdr, err := l.conn.ReadFrom(buf[:])
	if err != nil {
		return
	}
	sender, _ = sdr.(*net.UDPAddr)
	req, err = http.ReadRequest(bufio.NewReader(bytes.NewReader(buf[:n])))
	return
}

func (l *listener) handle(sender *net.UDPAddr, req *http.Request, log logging.Logger) {
	log = log.With(
		zap.Namespace("request"),
		"remote", sender.String(),
		"method", req.Method,
		"url", req.URL.String(),
		"headers", req.Header,
	)
	if req.Method != "M-SEARCH" || req.URL.String() != "*" || req.Header.Get("MAN") != `"ssdp:discover"` {
		log.Debugw("ignored request")
		return
	}

	conn, err := l.openReplyConn(sender, req.Header.Get("TCPPORT.UPNP.ORG"), log)
	if err != nil {
		log.Warnf("could not open reply connection: %s", err.Error())
		return
	}
	log = log.With(
		zap.Namespace("response"),
		"local", conn.LocalAddr().String(),
		"remote", conn.RemoteAddr().String(),
		"net", conn.LocalAddr().Network(),
	)

	maxDelay := readMaxDelay(req.Header, log)
	sts := l.resolveST(req.Header.Get("ST"))
	if len(sts) == 0 {
		log.Debugf("no notification types matching %q", req.Header.Get("ST"))
		return
	}
	maxDelay = maxDelay / time.Duration(len(sts))

	for _, st := range sts {
		l.sendResponse(conn, st, maxDelay, log.With("st", st))
	}
}

func (l *listener) openReplyConn(sender *net.UDPAddr, tcpPortHeader string, log logging.Logger) (conn net.Conn, err error) {
	ip, err := l.findLocalIPFor(sender)
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
	return net.DialTCP("udp", &net.TCPAddr{IP: ip}, &net.TCPAddr{IP: sender.IP, Port: port})
}

func (l *listener) sendResponse(conn net.Conn, st string, maxDelay time.Duration, log logging.Logger) {
	delay := time.Duration(rand.Int63n(int64(maxDelay)))
	select {
	case <-time.After(delay):
	case <-l.done:
		return
	}
	_, err := l.writeResponse(conn, mustGetIP(conn.LocalAddr()), st)
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

func (l *listener) writeResponse(conn net.Conn, ip net.IP, st string) (int, error) {
	return fmt.Fprintf(
		conn,
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

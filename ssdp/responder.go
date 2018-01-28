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
	"golang.org/x/net/ipv4"
)

type Responder struct {
	SSDPConfig
	conn *ipv4.PacketConn
	done chan struct{}
	w    sync.WaitGroup
	l    logging.Logger
}

func NewResponder(c SSDPConfig, l logging.Logger) *Responder {
	return &Responder{SSDPConfig: c, l: l.Named("responder")}
}

func (r *Responder) String() string {
	if r.conn != nil {
		return fmt.Sprintf("SSDP responder on %s", r.conn.LocalAddr().String())
	}
	return fmt.Sprintf("SSDP responder")
}

func (r *Responder) Serve() {
	conn, err := r.makeConn()
	if err != nil {
		r.l.Errorf("could not open connection: %s", err.Error())
		return
	}
	r.conn = conn
	defer conn.Close()
	r.done = make(chan struct{})
	r.w.Add(1)
	defer func() {
		r.done = nil
		r.w.Done()
	}()
	l := r.l.With("socket", conn.LocalAddr().String())

	for {
		msg := make([]byte, 2048)
		n, _, sender, err := r.conn.ReadFrom(msg)
		select {
		case <-r.done:
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
		go r.handle(sender.(*net.UDPAddr), msg[:n], l.With("client", sender.String()))
	}
}

func (r *Responder) Stop() {
	close(r.done)
	r.conn.Close()
	r.w.Wait()
}

func (r *Responder) makeConn() (conn *ipv4.PacketConn, err error) {
	ifaces, err := r.Interfaces()
	if err != nil {
		return
	}
	ip := net.IPv4(0, 0, 0, 0)
	if len(ifaces) == 1 {
		ip, _ = getUnicastAddr(&ifaces[0])
	}
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: ip, Port: 1900})
	if err != nil {
		return
	}
	conn = ipv4.NewPacketConn(c)
	for _, iface := range ifaces {
		if iErr := conn.JoinGroup(&iface, NetAddr); iErr != nil {
			r.l.Infof("listening on %s", iface.Name)
		} else {
			r.l.Errorf("could not join multicast group on %s: %s", iface.Name, iErr.Error())
		}
	}
	return
}

func getUnicastAddr(iface *net.Interface) (ip net.IP, err error) {
	addrs, err := iface.Addrs()
	if err != nil {
		return
	}
	for _, addr := range addrs {
		var i net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			i = v.IP
		case *net.IPAddr:
			i = v.IP
		default:
			continue
		}
		if i.IsMulticast() || i.To4() == nil {
			continue
		}
		ip = i
		return
	}
	return nil, nil
}

func (r *Responder) handle(sender *net.UDPAddr, msg []byte, l logging.Logger) {
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(msg)))
	if err != nil {
		l.Errorf("cannot read requests: %s", err.Error())
		return
	}

	if req.Method != "M-SEARCH" || req.URL.String() != "*" || req.Header.Get("Man") != `"ssdp:discover"` {
		l.Debugf("ignored request, Method=%q URL=%q Man=%q", req.Method, req.URL.String(), req.Header.Get("Man"))
		return
	}

	if tcpPort := req.Header.Get("TCPPORT.UPNP.ORG"); tcpPort != "" {
		l.Warnf("ignored M-SEARCH, cannot reply to TCP port %s", tcpPort)
		return
	}

	ip, err := r.findLocalIPFor(sender)
	if err != nil {
		l.Errorf("could not find a local addr to reply to %s: %s", err.Error())
		return
	}

	maxDelay := readMaxDelay(req.Header.Get("Mx"), l)
	sts := r.resolveST(req.Header.Get("St"))
	if len(sts) == 0 {
		l.Debugf("no matching notification types", req.Header.Get("St"))
		return
	}
	maxDelay = maxDelay / time.Duration(len(sts))

	for _, st := range sts {
		msg := r.makeResponse(ip, st)
		delay := time.Duration(rand.Int63n(int64(maxDelay)))
		select {
		case <-time.After(delay):
		case <-r.done:
			return
		}
		if n, err := r.conn.WriteTo(msg, nil, sender); err != nil {
			l.Errorf("could not send: %s", err.Error())
		} else if n < len(msg) {
			l.Errorf("short write: %d/%d", n, len(msg))
		} else {
			l.Debugf("%q response sent", st)
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

func (r *Responder) makeResponse(ip net.IP, st string) []byte {
	s := fmt.Sprintf(
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
	return []byte(s)
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

package ssdp

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/net/ipv4"
)

type Responder struct {
	SSDPConfig
	conn *ipv4.PacketConn
	done chan struct{}
	w    sync.WaitGroup
}

func NewResponder(c SSDPConfig) *Responder {
	return &Responder{SSDPConfig: c}
}

func (r *Responder) Serve() {
	conn, err := r.makeConn()
	if err != nil {
		log.Panicf("could not open connection: %s", err)
	}
	r.conn = conn
	r.done = make(chan struct{})
	r.w.Add(1)
	defer func() {
		r.done = nil
		r.w.Done()
	}()

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
				log.Printf("error while receiving: %s", err.Error())
				continue
			}
			log.Panicf("error while receiving: %s", err.Error())
		}
		go r.handle(sender.(*net.UDPAddr), msg[:n])
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
	c, err := net.ListenPacket("udp4", "0.0.0.0:1900")
	if err != nil {
		return
	}
	conn = ipv4.NewPacketConn(c)
	for _, iface := range ifaces {
		if iface.Name == "" {
			continue
		}
		log.Printf("%#v", iface)
		if iErr := conn.JoinGroup(&iface, NetAddr); iErr != nil {
			log.Printf("could not join multicast group on %s: %s", iface.Name, iErr.Error())
		}
	}
	return
}

func (r *Responder) handle(sender *net.UDPAddr, msg []byte) {
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(msg)))
	if err != nil {
		log.Printf("cannot read request from %s: %s", sender.String(), err.Error())
		return
	}

	if req.Method != "M-SEARCH" || req.URL.String() != "*" || req.Header.Get("Man") != `"ssdp:discover"` {
		log.Printf("ignored request from %s, Method=%q URL=%q Man=%q", sender.String(), req.Method, req.URL.String(), req.Header.Get("Man"))
		return
	}

	if tcpPort := req.Header.Get("TCPPORT.UPNP.ORG"); tcpPort != "" {
		log.Printf("ignored M-SEARCH from %s, cannot reply to TCP port %s", sender.String(), tcpPort)
		return
	}

	ip, err := r.findLocalIPFor(sender)
	if err != nil {
		log.Printf("could not find a local addr to reply to %s: %s", sender.String(), err.Error())
		return
	}

	maxDelay := readMaxDelay(req.Header.Get("Mx"))
	sts := r.resolveST(req.Header.Get("St"))
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
			log.Printf("could not send to %s: %s", sender.String(), err.Error())
		} else if n < len(msg) {
			log.Printf("short write to %s: %d/%d", sender.String(), n, len(msg))
		} else {
			log.Printf("%q response sent to %s", st, sender.String())
		}
	}
}

func readMaxDelay(mx string) time.Duration {
	if mx == "" {
		return time.Second
	}
	n, err := strconv.Atoi(mx)
	if err != nil {
		log.Printf("invalid mx: %q", mx)
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

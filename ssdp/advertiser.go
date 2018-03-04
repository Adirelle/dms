package ssdp

import (
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/Adirelle/go-libs/logging"
	"golang.org/x/net/ipv4"
)

const (
	flagSSDP  = net.FlagUp | net.FlagMulticast
	aliveNTS  = "ssdp:alive"
	byebyeNTS = "ssdp:byebye"
)

type Advertiser struct {
	Config
	responderPort func() int
	done          chan struct{}
	w             sync.WaitGroup
	l             logging.Logger
}

func NewAdvertiser(c Config, rp func() int, l logging.Logger) *Advertiser {
	return &Advertiser{Config: c, responderPort: rp, l: l}
}

func (a *Advertiser) String() string {
	return "ssdp.advertiser"
}

func (a *Advertiser) Serve() {
	a.done = make(chan struct{})
	a.w.Add(1)
	defer func() {
		a.done = nil
		a.notifyAll(byebyeNTS, true)
		a.w.Done()
	}()
	for {
		a.l.Info("announcing")
		go a.notifyAll(aliveNTS, false)
		select {
		case <-time.After(a.NotifyInterval):
		case <-a.done:
			return
		}
	}
}

func (a *Advertiser) Stop() {
	close(a.done)
	a.w.Wait()
	a.l.Info("stopped")
}

func (a *Advertiser) notifyAll(nts string, immediate bool) {
	log := a.l.With(zap.Namespace("notification"), "nts", nts)
	ifaces, err := a.Interfaces()
	if err != nil {
		log.Errorf("could not get interfaces: %s", err.Error())
		return
	}
	wg := sync.WaitGroup{}
	for _, iface := range ifaces {
		if iface.Flags&flagSSDP != flagSSDP {
			continue
		}
		go func(iface *net.Interface) {
			defer wg.Done()
			a.notifyIFace(iface, nts, immediate, log.With("iface", iface.Name))
		}(&iface)
		wg.Add(1)
	}
	wg.Wait()
}

func (a *Advertiser) notifyIFace(iface *net.Interface, nts string, immediate bool, log logging.Logger) {
	ips, err := a.getMulticastSourceAddrs(iface)
	if err != nil {
		log.Warnf("cannot multicast using %s: %s", iface.Name, err.Error())
		return
	}
	for _, ip := range ips {
		a.notify(ip, nts, immediate, log.With("local", ip.String()))
	}
}

func (a *Advertiser) getMulticastSourceAddrs(iface *net.Interface) (ips []net.IP, err error) {
	ifAddrs, err := iface.Addrs()
	if err != nil {
		return
	}
	ips = make([]net.IP, 0, len(ifAddrs))
	for _, addr := range ifAddrs {
		var ip net.IP
		switch val := addr.(type) {
		case *net.IPNet:
			ip = val.IP
		case *net.IPAddr:
			ip = val.IP
		}
		if ip != nil && ip.To4() != nil {
			ips = append(ips, ip)
		}
	}
	if len(ips) == 0 {
		err = fmt.Errorf("no source address")
	}
	return
}

func (a *Advertiser) notify(ip net.IP, nts string, immediate bool, log logging.Logger) {
	conn, err := a.openConn(ip)
	if err != nil {
		log.Warnf("cannot multicast: %s", err.Error())
		return
	}
	defer conn.Close()
	for _, nt := range a.allTypes() {
		if !immediate {
			delay := time.Duration(rand.Int63n(int64(100 * time.Millisecond)))
			select {
			case <-time.After(delay):
			case <-a.done:
				return
			}
		}
		a.notifyType(conn, nt, nts, log.With("nt", nt))
	}
}

func (a *Advertiser) notifyType(conn net.Conn, nt, nts string, log logging.Logger) {
	_, err := a.writeNotification(conn, nt, nts)
	if err != nil {
		log.Warnf("could not send notification: %s", err.Error())
	} else {
		log.Debug("notification sent")
	}
}

func (a *Advertiser) openConn(ip net.IP) (conn *net.UDPConn, err error) {
	conn, err = net.DialUDP("udp", &net.UDPAddr{IP: ip}, NetAddr)
	if err != nil {
		return
	}
	p := ipv4.NewPacketConn(conn)
	if err = p.SetMulticastTTL(2); err == nil {
		err = p.SetMulticastLoopback(true)
	}
	return
}

const notifyTpl = "NOTIFY * HTTP/1.1\r\n" +
	"HOST: %s\r\n" +
	"NT: %s\r\n" +
	"NTS: %s\r\n" +
	"USN: %s\r\n" +
	"LOCATION: %s\r\n" +
	"DATE: %s\r\n" +
	"CACHE-CONTROL: max-age=%d\r\n" +
	"EXT:\r\n" +
	"SERVER: %s\r\n" +
	"BOOTID.UPNP.ORG: %d\r\n" +
	"CONFIGID.UPNP.ORG: %d\r\n" +
	"SEARCHPORT.UPNP.ORG: %d\r\n" +
	"\r\n"

func (a *Advertiser) writeNotification(conn net.Conn, nt, nts string) (int, error) {
	return fmt.Fprintf(
		conn,
		notifyTpl,
		AddrString,
		nt,
		nts,
		a.usnFromTarget(nt),
		a.Location(conn.LocalAddr().(*net.UDPAddr).IP),
		time.Now().Format(time.RFC1123),
		5*a.NotifyInterval/2/time.Second,
		a.Server,
		a.BootID,
		a.ConfigID,
		a.responderPort(),
	)
}

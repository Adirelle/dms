package ssdp

import (
	"fmt"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/anacrolix/dms/logging"
	"golang.org/x/net/ipv4"
)

const (
	flagSSDP  = net.FlagUp | net.FlagMulticast
	aliveNTS  = "ssdp:alive"
	byebyeNTS = "ssdp:byebye"
)

type Advertiser struct {
	SSDPConfig
	done chan struct{}
	w    sync.WaitGroup
	l    logging.Logger
}

func NewAdvertiser(c SSDPConfig, l logging.Logger) *Advertiser {
	return &Advertiser{SSDPConfig: c, l: l.Named("advertiser")}
}

}

func (a *Advertiser) Serve() {
	a.done = make(chan struct{})
	a.w.Add(1)
	defer func() {
		a.done = nil
		a.notifyAll(byebyeNTS, true)
		a.w.Done()
		a.l.Info("stopped advertiser")
	}()
	a.l.Info("started advertiser")
	for {
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
}

func (a *Advertiser) notifyAll(nts string, immediate bool) {
	ifaces, err := a.Interfaces()
	if err != nil {
		a.l.Errorf("could not get interfaces: %s", err.Error())
		return
	}
	wg := sync.WaitGroup{}
	for _, iface := range ifaces {
		if iface.Flags&flagSSDP != flagSSDP {
			continue
		}
		go func(iface *net.Interface) {
			defer wg.Done()
			a.notifyIFace(iface, nts, immediate)
		}(&iface)
		wg.Add(1)
	}
	wg.Wait()
}

func (a *Advertiser) notifyIFace(iface *net.Interface, nts string, immediate bool) {
	ips, err := a.getMulticastSourceAddrs(iface)
	if err != nil {
		a.l.Errorf("cannot multicast using %s: %s", iface.Name, err.Error())
		return
	}
	for _, ip := range ips {
		a.notify(ip, nts, immediate)
	}
}

func (a *Advertiser) getMulticastSourceAddrs(iface *net.Interface) (ips []net.IP, err error) {
	ifAddrs, err := iface.Addrs()
	if err != nil {
		return
	}
	ips = make([]net.IP, 0)
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

func (a *Advertiser) notify(ip net.IP, nts string, immediate bool) {
	l := a.l.With("source", ip.String())
	conn, err := a.openConn(ip)
	if err != nil {
		l.Errorf("cannot multicast %s", err.Error())
		return
	}
	defer conn.Close()
	for _, msgType := range a.allTypes() {
		if !immediate {
			delay := time.Duration(rand.Int63n(int64(100 * time.Millisecond)))
			select {
			case <-time.After(delay):
			case <-a.done:
				return
			}
		}
		buf := a.makeNotifyMessage(msgType, nts, ip)
		n, err := conn.Write(buf)
		if err != nil {
			l.Errorf("could not send notify: %s", err.Error())
		} else if n < len(buf) {
			l.Errorf("short write %d/%d", n, len(buf))
		} else {
			l.Debugf("%s %q notify sent to %s", nts, msgType, conn.RemoteAddr().String())
		}
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
	"\r\n"

func (a *Advertiser) makeNotifyMessage(target, nts string, ip net.IP) []byte {
	msg := fmt.Sprintf(
		notifyTpl,
		AddrString,
		target,
		nts,
		a.usnFromTarget(target),
		a.Location(ip),
		time.Now().Format(time.RFC1123),
		5*a.NotifyInterval/2/time.Second,
		a.Server,
		a.BootID,
		a.ConfigID,
	)
	return []byte(msg)
}

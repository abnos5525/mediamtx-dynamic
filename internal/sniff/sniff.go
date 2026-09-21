package sniff

import (
	"log"
	"net"
	"net/url"
	"time"

	"github.com/abnos5525/mediamtx-dynamic/internal/ai"
	"github.com/abnos5525/mediamtx-dynamic/internal/rtp"
)

// Run joins multicast and pushes AI frames only (video is owned by MediaMTX).
func Run(rawURL string, onFrame func(*ai.AIFrame), stop <-chan struct{}) {
	u := rtp.MulticastURL(rawURL)
	if u == nil {
		u, _ = url.Parse(rawURL)
	}
	if u == nil {
		return
	}
	ip := net.ParseIP(u.Hostname())
	buf := make([]byte, 65535)
	var fu []byte
	var mc *net.UDPConn
	defer func() {
		if mc != nil {
			_ = mc.Close()
		}
	}()

	for {
		select {
		case <-stop:
			return
		default:
		}
		if mc == nil {
			c, err := rtp.JoinMulticast(u)
			if err != nil {
				log.Printf("ai sniff join %s: %v", u.Host, err)
				time.Sleep(time.Second)
				continue
			}
			mc = c
		}
		_ = mc.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, err := mc.Read(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				rtp.Rejoin(mc, ip)
				continue
			}
			log.Printf("ai sniff %s: %v", u.Host, err)
			_ = mc.Close()
			mc = nil
			select {
			case <-stop:
				return
			case <-time.After(200 * time.Millisecond):
			}
			continue
		}
		extract(buf[:n], &fu, onFrame)
	}
}

func extract(pkt []byte, fu *[]byte, onFrame func(*ai.AIFrame)) {
	pt, _, payload, ext, ok := rtp.Parse(pkt)
	if !ok {
		if f := ai.Parse(pkt); f != nil {
			onFrame(f)
		}
		return
	}
	if len(ext) > 0 {
		if f := ai.FromExt(ext); f != nil {
			onFrame(f)
		}
	}
	if f := ai.Parse(payload); f != nil {
		onFrame(f)
	}
	if pt != 96 {
		return
	}
	for _, nal := range rtp.NALs(payload, fu) {
		if f := ai.FromNAL(nal); f != nil {
			onFrame(f)
		}
	}
}

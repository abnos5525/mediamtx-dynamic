package session

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"sync"

	"github.com/abnos5525/mediamtx-dynamic/internal/ai"
	"github.com/abnos5525/mediamtx-dynamic/internal/mtx"
	"github.com/abnos5525/mediamtx-dynamic/internal/overlay"
	"github.com/abnos5525/mediamtx-dynamic/internal/sniff"
)

var pathSafe = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

type Manager struct {
	MTX        *mtx.Client
	HLSBase    string
	WebRTCBase string
	Overlay    *overlay.Store

	mu    sync.Mutex
	byURL map[string]*Sess
	byID  map[string]*Sess
}

type Sess struct {
	ID     string
	URL    string
	Format string
	stop   chan struct{}
}

func New(m *mtx.Client, hls, webrtc string, ov *overlay.Store) *Manager {
	return &Manager{
		MTX: m, HLSBase: hls, WebRTCBase: webrtc, Overlay: ov,
		byURL: map[string]*Sess{}, byID: map[string]*Sess{},
	}
}

func ParseIngest(raw string) (host, port, name string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", "", fmt.Errorf("url required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", "", err
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "udp" && scheme != "rtp" {
		return "", "", "", fmt.Errorf("scheme must be udp or rtp")
	}
	host = u.Hostname()
	port = u.Port()
	if host == "" || port == "" {
		return "", "", "", fmt.Errorf("need host:port")
	}
	if net.ParseIP(host) == nil {
		return "", "", "", fmt.Errorf("invalid host ip")
	}
	p := strings.Trim(u.Path, "/")
	if p != "" {
		name = pathSafe.ReplaceAllString(p, "_")
	} else {
		name = pathSafe.ReplaceAllString(host+"_"+port, "_")
	}
	if name == "" {
		return "", "", "", fmt.Errorf("empty path name")
	}
	return host, port, name, nil
}

type StartOut struct {
	URL        string `json:"url"`
	OutputType int    `json:"output_type"`
	ID         string `json:"id"`
	HLS        string `json:"hls"`
	WebRTC     string `json:"webrtc"`
	RTSP       string `json:"rtsp"`
	Source     string `json:"source,omitempty"`
}

func (m *Manager) Start(rawURL, format string, outputType int) (*StartOut, error) {
	host, port, id, err := ParseIngest(rawURL)
	if err != nil {
		return nil, err
	}
	if format == "" {
		format = "rtp"
	}
	body, err := mtx.PathBody(host, port, format)
	if err != nil {
		return nil, err
	}
	if err := m.MTX.Upsert(id, body); err != nil {
		return nil, err
	}

	m.mu.Lock()
	if old, ok := m.byURL[rawURL]; ok {
		m.mu.Unlock()
		return m.urls(old, outputType, fmt.Sprint(body["source"])), nil
	}
	s := &Sess{ID: id, URL: rawURL, Format: format, stop: make(chan struct{})}
	m.byURL[rawURL] = s
	m.byID[id] = s
	m.mu.Unlock()

	go sniff.Run(rawURL, func(f *ai.AIFrame) {
		m.Overlay.Push(id, f)
	}, s.stop)

	src, _ := body["source"].(string)
	return m.urls(s, outputType, src), nil
}

func (m *Manager) Stop(rawURL, id string) error {
	m.mu.Lock()
	var s *Sess
	if id != "" {
		s = m.byID[id]
	}
	if s == nil && rawURL != "" {
		s = m.byURL[rawURL]
		if s == nil {
			_, _, n, err := ParseIngest(rawURL)
			if err == nil {
				s = m.byID[n]
				id = n
			}
		}
	}
	if s != nil {
		id = s.ID
		delete(m.byURL, s.URL)
		delete(m.byID, s.ID)
		close(s.stop)
	}
	m.mu.Unlock()
	if id == "" {
		return fmt.Errorf("unknown stream")
	}
	m.Overlay.Clear(id)
	return m.MTX.Delete(id)
}

func (m *Manager) List() []*Sess {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*Sess, 0, len(m.byID))
	for _, s := range m.byID {
		out = append(out, s)
	}
	return out
}

func (m *Manager) Get(id string) *Sess {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.byID[id]
}

func (m *Manager) urls(s *Sess, outputType int, source string) *StartOut {
	hls := strings.TrimRight(m.HLSBase, "/") + "/" + s.ID + "/index.m3u8"
	webrtc := strings.TrimRight(m.WebRTCBase, "/") + "/" + s.ID + "/whep"
	out := &StartOut{
		ID: s.ID, HLS: hls, WebRTC: webrtc,
		RTSP: "rtsp://127.0.0.1:8554/" + s.ID, Source: source,
	}
	if outputType == 7 {
		out.URL, out.OutputType = webrtc, 7
	} else {
		out.URL, out.OutputType = hls, 6
	}
	return out
}

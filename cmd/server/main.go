// Thin API in front of MediaMTX Control API.
// Accepts Hafez-style urls:
//
//	udp://224.2.0.1:5493/restream/<id>_720
//	rtsp://user:pass@camera/stream
//
// UDP/RTP joins multicast host:port as RTP (PT 96 / H264).
// RTSP is pulled by MediaMTX over TCP. HLS + WebRTC are MediaMTX, not this process.
package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/abnos5525/mediamtx-dynamic/internal/swagger"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

var (
	listenAddr = env("LISTEN", ":8090")
	mtxAPI     = env("MTX_API", "http://127.0.0.1:9997")
	hlsBase    = env("HLS_BASE", "http://127.0.0.1:8888")
	webrtcBase = env("WEBRTC_BASE", "http://127.0.0.1:8889")
	readyWait  = 8 * time.Second
)

type startReq struct {
	URL        string `json:"url"`
	OutputType int    `json:"output_type"` // 6=HLS, 7=WebRTC
}

type startResp struct {
	URL        string `json:"url"`
	OutputType int    `json:"output_type"`
	Path       string `json:"path"`
	HLS        string `json:"hls,omitempty"`
	WebRTC     string `json:"webrtc,omitempty"`
}

type ingest struct {
	raw  string
	host string
	port string
	name string
	rtsp bool
}

var (
	mu    sync.Mutex
	paths = map[string]string{} // ingestURL -> mtx path name
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /stream/start", handleStart)
	mux.HandleFunc("POST /stream/stop", handleStop)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /openapi.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(swagger.Spec())
	})
	mux.HandleFunc("GET /api/doc", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/api/doc/", http.StatusFound)
	})
	mux.Handle("GET /api/doc/", swagger.UI())

	log.Printf("api on %s (MediaMTX %s)", listenAddr, mtxAPI)
	log.Print("swagger at http://127.0.0.1" + listenAddr + "/api/doc/")
	log.Fatal(http.ListenAndServe(listenAddr, withCORS(mux)))
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", "*")
		h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		h.Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func handleStart(w http.ResponseWriter, r *http.Request) {
	var req startReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.OutputType != 6 && req.OutputType != 7 {
		http.Error(w, "output_type must be 6 or 7", http.StatusBadRequest)
		return
	}
	in, err := parseIngest(req.URL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	mu.Lock()
	_, exists := paths[in.raw]
	if !exists {
		paths[in.raw] = in.name
	}
	name := paths[in.raw]
	mu.Unlock()

	if !exists {
		if err := mtxAddPath(in); err != nil {
			mu.Lock()
			delete(paths, in.raw)
			mu.Unlock()
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
	}
	if err := waitReady(name); err != nil {
		if !exists {
			mu.Lock()
			delete(paths, in.raw)
			mu.Unlock()
			_ = mtxDeletePath(name)
		}
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	writeStart(w, name, req.OutputType)
}

func handleStop(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	mu.Lock()
	name, ok := paths[req.URL]
	if ok {
		delete(paths, req.URL)
	}
	mu.Unlock()

	if !ok {
		in, err := parseIngest(req.URL)
		if err != nil {
			http.Error(w, "unknown stream", http.StatusNotFound)
			return
		}
		name = in.name
	}
	if err := mtxDeletePath(name); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"message":"stopped"}`))
}

func writeStart(w http.ResponseWriter, pathName string, outputType int) {
	out := startResp{Path: pathName, OutputType: outputType}
	if outputType == 7 {
		out.URL = webrtcBase + "/" + pathName + "/whep"
		out.WebRTC = out.URL
	} else {
		out.URL = hlsBase + "/" + pathName + "/index.m3u8"
		out.HLS = out.URL
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func parseIngest(raw string) (ingest, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ingest{}, fmt.Errorf("url required")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ingest{}, fmt.Errorf("url must be rtsp://, rtsps://, udp:// or rtp://")
	}
	sum := sha1.Sum([]byte(raw))
	in := ingest{raw: raw, name: "s" + hex.EncodeToString(sum[:8])}
	switch strings.ToLower(u.Scheme) {
	case "rtsp", "rtsps":
		in.rtsp = true
		return in, nil
	case "udp", "rtp":
		host, port, err := net.SplitHostPort(u.Host)
		if err != nil {
			return ingest{}, fmt.Errorf("need host:port: %w", err)
		}
		if net.ParseIP(host) == nil {
			return ingest{}, fmt.Errorf("invalid host ip")
		}
		in.host, in.port = host, port
		return in, nil
	default:
		return ingest{}, fmt.Errorf("url must be rtsp://, rtsps://, udp:// or rtp://")
	}
}

func mtxAddPath(in ingest) error {
	var body []byte
	if in.rtsp {
		body, _ = json.Marshal(map[string]any{
			"source":         in.raw,
			"sourceOnDemand": false,
			"rtspTransport":  "tcp",
		})
	} else {
		sdp := fmt.Sprintf("v=0\no=- 0 0 IN IP4 %s\ns=dynamic\nc=IN IP4 %s\nt=0 0\nm=video %s RTP/AVP 96\na=rtpmap:96 H264/90000\na=fmtp:96 packetization-mode=1\n",
			in.host, in.host, in.port)
		body, _ = json.Marshal(map[string]any{
			"source":         "udp+rtp://" + net.JoinHostPort(in.host, in.port),
			"rtpSDP":         sdp,
			"sourceOnDemand": false,
		})
	}
	res, err := mtxPost("/v3/config/paths/add/"+in.name, body)
	if err != nil {
		return fmt.Errorf("MediaMTX API: %w (is mediamtx running?)", err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode == http.StatusOK {
		return nil
	}
	if res.StatusCode == http.StatusBadRequest && bytes.Contains(b, []byte("already exists")) {
		return mtxReplacePath(in.name, body)
	}
	return fmt.Errorf("MediaMTX add path %s: %s %s", in.name, res.Status, string(b))
}

func mtxReplacePath(name string, body []byte) error {
	res, err := mtxPost("/v3/config/paths/replace/"+name, body)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("MediaMTX replace path: %s %s", res.Status, string(b))
	}
	return nil
}

func mtxDeletePath(name string) error {
	res, err := mtxPost("/v3/config/paths/delete/"+name, nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusNotFound {
		b, _ := io.ReadAll(res.Body)
		return fmt.Errorf("MediaMTX delete path: %s %s", res.Status, string(b))
	}
	return nil
}

func waitReady(name string) error {
	deadline := time.Now().Add(readyWait)
	var last error
	for {
		ready, err := pathReady(name)
		if err != nil {
			last = err
		} else if ready {
			return nil
		}
		if time.Now().After(deadline) {
			if last != nil {
				return fmt.Errorf("stream not ready: %w", last)
			}
			return fmt.Errorf("stream not ready")
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func pathReady(name string) (bool, error) {
	req, err := http.NewRequest(http.MethodGet, mtxAPI+"/v3/paths/get/"+url.PathEscape(name), nil)
	if err != nil {
		return false, err
	}
	res, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		return false, err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if res.StatusCode != http.StatusOK {
		return false, fmt.Errorf("MediaMTX path: %s %s", res.Status, string(b))
	}
	var got struct {
		Ready bool `json:"ready"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		return false, err
	}
	return got.Ready, nil
}

func mtxPost(path string, body []byte) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequest(http.MethodPost, mtxAPI+path, r)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return (&http.Client{Timeout: 10 * time.Second}).Do(req)
}

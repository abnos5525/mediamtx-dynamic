// Thin API in front of MediaMTX Control API.
// Accepts Hafez-style urls:
//
//	udp://224.2.0.1:5493/restream/<id>_720
//
// Joins multicast host:port as RTP (PT 96 / H264) and exposes HLS + WebRTC.
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
)

type startReq struct {
	URL        string `json:"url"`
	OutputType int    `json:"output_type"` // 6=HLS (default), 7=WebRTC
}

type startResp struct {
	URL        string `json:"url"`
	OutputType int    `json:"output_type"`
	Path       string `json:"path"`
	HLS        string `json:"hls,omitempty"`
	WebRTC     string `json:"webrtc,omitempty"`
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

	log.Printf("api on %s (MediaMTX %s)", listenAddr, mtxAPI)
	log.Fatal(http.ListenAndServe(listenAddr, mux))
}

func handleStart(w http.ResponseWriter, r *http.Request) {
	var req startReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	host, port, pathName, err := parseIngest(req.URL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	mu.Lock()
	defer mu.Unlock()
	if existing, ok := paths[req.URL]; ok {
		writeStart(w, existing, req.OutputType)
		return
	}
	if err := mtxAddPath(pathName, host, port); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	paths[req.URL] = pathName
	writeStart(w, pathName, req.OutputType)
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
		_, _, pathName, err := parseIngest(req.URL)
		if err != nil {
			http.Error(w, "unknown stream", http.StatusNotFound)
			return
		}
		name = pathName
	}
	if err := mtxDeletePath(name); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"message":"stopped"}`))
}

func writeStart(w http.ResponseWriter, pathName string, outputType int) {
	hls := hlsBase + "/" + pathName + "/index.m3u8"
	webrtc := webrtcBase + "/" + pathName + "/whep"
	out := startResp{Path: pathName, HLS: hls, WebRTC: webrtc}
	if outputType == 7 {
		out.URL, out.OutputType = webrtc, 7
	} else {
		out.URL, out.OutputType = hls, 6
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func parseIngest(raw string) (host, port, pathName string, err error) {
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
	host, port, err = net.SplitHostPort(u.Host)
	if err != nil {
		return "", "", "", fmt.Errorf("need host:port: %w", err)
	}
	if net.ParseIP(host) == nil {
		return "", "", "", fmt.Errorf("invalid host ip")
	}
	sum := sha1.Sum([]byte(raw))
	return host, port, "s" + hex.EncodeToString(sum[:8]), nil
}

func mtxAddPath(name, host, port string) error {
	sdp := fmt.Sprintf("v=0\no=- 0 0 IN IP4 %s\ns=dynamic\nc=IN IP4 %s\nt=0 0\nm=video %s RTP/AVP 96\na=rtpmap:96 H264/90000\na=fmtp:96 packetization-mode=1\n",
		host, host, port)
	body, _ := json.Marshal(map[string]any{
		"source": "udp+rtp://" + net.JoinHostPort(host, port),
		"rtpSDP": sdp,
	})
	res, err := mtxPost("/v3/config/paths/add/"+name, body)
	if err != nil {
		return fmt.Errorf("MediaMTX API: %w (is mediamtx running?)", err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode == http.StatusOK {
		return nil
	}
	if res.StatusCode == http.StatusBadRequest && bytes.Contains(b, []byte("already exists")) {
		return mtxReplacePath(name, body)
	}
	return fmt.Errorf("MediaMTX add path %s: %s %s", name, res.Status, string(b))
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

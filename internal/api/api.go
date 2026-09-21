package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/abnos5525/mediamtx-dynamic/internal/mtx"
	"github.com/abnos5525/mediamtx-dynamic/internal/overlay"
	"github.com/abnos5525/mediamtx-dynamic/internal/session"
)

type Server struct {
	Mgr *session.Manager
	MTX *mtx.Client
	OV  *overlay.Store
}

func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("POST /stream/start", s.start)
	mux.HandleFunc("POST /stream/stop", s.stop)
	mux.HandleFunc("GET /stream/meta", s.metaAll)
	mux.HandleFunc("GET /stream/meta/{id}", s.metaOne)
	mux.HandleFunc("GET /stream/overlay/{id}", s.overlay)
}

func (s *Server) start(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL        string `json:"url"`
		OutputType int    `json:"output_type"`
		Format     string `json:"format"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.URL == "" {
		http.Error(w, `{"message":"url required"}`, http.StatusBadRequest)
		return
	}
	out, err := s.Mgr.Start(req.URL, req.Format, req.OutputType)
	if err != nil {
		code := http.StatusBadRequest
		if strings.Contains(err.Error(), "MediaMTX") {
			code = http.StatusBadGateway
		}
		writeErr(w, code, err.Error())
		return
	}
	writeJSON(w, out)
}

func (s *Server) stop(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL string `json:"url"`
		ID  string `json:"id"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if err := s.Mgr.Stop(req.URL, req.ID); err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, map[string]string{"message": "stopped"})
}

type metaOut struct {
	ID         string  `json:"id"`
	URL        string  `json:"url"`
	RTSP       string  `json:"rtsp"`
	Started    bool    `json:"started"`
	Ready      bool    `json:"ready"`
	AIAgeMS    int64   `json:"ai_age_ms,omitempty"`
	AIFPS      float64 `json:"ai_fps,omitempty"`
	OutputType int     `json:"output_type,omitempty"`
	HLS        string  `json:"hls,omitempty"`
	WebRTC     string  `json:"webrtc,omitempty"`
}

func (s *Server) metaAll(w http.ResponseWriter, _ *http.Request) {
	list := s.Mgr.List()
	out := make([]metaOut, 0, len(list))
	for _, sess := range list {
		out = append(out, s.metaOf(sess))
	}
	writeJSON(w, out)
}

func (s *Server) metaOne(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sess := s.Mgr.Get(id)
	if sess == nil {
		writeErr(w, http.StatusNotFound, "unknown stream")
		return
	}
	writeJSON(w, s.metaOf(sess))
}

func (s *Server) metaOf(sess *session.Sess) metaOut {
	m := metaOut{
		ID: sess.ID, URL: sess.URL, Started: true, OutputType: 6,
		RTSP:   "rtsp://127.0.0.1:8554/" + sess.ID,
		HLS:    strings.TrimRight(s.Mgr.HLSBase, "/") + "/" + sess.ID + "/index.m3u8",
		WebRTC: strings.TrimRight(s.Mgr.WebRTCBase, "/") + "/" + sess.ID + "/whep",
	}
	if p, err := s.MTX.GetPath(sess.ID); err == nil && p != nil {
		m.Ready = p.Ready
	}
	if age, fps, ok := s.OV.MetaAI(sess.ID); ok {
		m.AIAgeMS, m.AIFPS = age, fps
	}
	return m
}

func (s *Server) overlay(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	lag := int64(-1)
	if v := r.URL.Query().Get("lag_ms"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			lag = n
		}
	}
	view, ok := s.OV.Get(id, lag)
	if !ok {
		// empty overlay so FE can poll before first AI frame
		view = overlay.View{ID: id, Boxes: []overlay.Box{}}
	}
	writeJSON(w, view)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": msg})
}

func CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

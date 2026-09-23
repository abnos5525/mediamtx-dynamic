package main

import (
	"log"
	"net/http"
	"os"

	"github.com/abnos5525/mediamtx-dynamic/internal/api"
	"github.com/abnos5525/mediamtx-dynamic/internal/mtx"
	"github.com/abnos5525/mediamtx-dynamic/internal/overlay"
	"github.com/abnos5525/mediamtx-dynamic/internal/session"
	"github.com/abnos5525/mediamtx-dynamic/internal/swagger"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	listen := env("LISTEN", ":8090")
	mtxAPI := env("MTX_API", "http://127.0.0.1:9997")
	hls := env("HLS_BASE", "http://127.0.0.1:8888")
	webrtc := env("WEBRTC_BASE", "http://127.0.0.1:8889")

	ov := overlay.NewStore()
	client := mtx.New(mtxAPI)
	mgr := session.New(client, hls, webrtc, ov)
	srv := &api.Server{Mgr: mgr, MTX: client, OV: ov}

	mux := http.NewServeMux()
	srv.Register(mux)
	swagger.Register(mux)

	log.Printf("mediamtx-dynamic api on %s (mtx=%s) swagger=/api/doc", listen, mtxAPI)
	log.Fatal(http.ListenAndServe(listen, api.CORS(mux)))
}

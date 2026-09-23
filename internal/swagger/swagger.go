package swagger

import (
	"encoding/json"
	"net/http"

	"github.com/swaggest/openapi-go"
	"github.com/swaggest/openapi-go/openapi3"
	"github.com/swaggest/swgui"
	"github.com/swaggest/swgui/v5emb"
)

type streamStart struct {
	URL        string `json:"url" example:"udp://224.2.0.1:5493/restream/cam_720"`
	OutputType int    `json:"output_type" example:"6"`
}

type streamStarted struct {
	URL        string `json:"url"`
	OutputType int    `json:"output_type"`
	Path       string `json:"path"`
	HLS        string `json:"hls,omitempty"`
	WebRTC     string `json:"webrtc,omitempty"`
}

type streamStop struct {
	URL string `json:"url" example:"udp://224.2.0.1:5493/restream/cam_720"`
}

type streamMsg struct {
	Message string `json:"message"`
}

func spec() []byte {
	r := openapi3.NewReflector()
	r.SpecEns().Info.
		WithTitle("mediamtx-dynamic").
		WithVersion("1.0.0").
		WithDescription("Start a MediaMTX path from a udp, rtp, rtsp, or rtsps URL. output_type 7 returns only the WebRTC WHEP url. output_type 6 returns only the LL-HLS url. The API waits until MediaMTX reports the path ready.")
	add(r, http.MethodPost, "/stream/start", "Start ingest", new(streamStart), new(streamStarted))
	add(r, http.MethodPost, "/stream/stop", "Stop ingest", new(streamStop), new(streamMsg))
	health, err := r.NewOperationContext(http.MethodGet, "/health")
	if err != nil {
		panic(err)
	}
	health.SetSummary("Process is up")
	health.AddRespStructure(new(string), func(cu *openapi.ContentUnit) { cu.HTTPStatus = http.StatusOK })
	if err := r.AddOperation(health); err != nil {
		panic(err)
	}
	b, err := json.Marshal(r.Spec)
	if err != nil {
		panic(err)
	}
	return b
}

func add(r *openapi3.Reflector, method, path, summary string, req, resp any) {
	op, err := r.NewOperationContext(method, path)
	if err != nil {
		panic(err)
	}
	op.SetSummary(summary)
	op.AddReqStructure(req)
	op.AddRespStructure(resp, func(cu *openapi.ContentUnit) { cu.HTTPStatus = http.StatusOK })
	if err := r.AddOperation(op); err != nil {
		panic(err)
	}
}

// Spec is the OpenAPI document.
func Spec() []byte { return spec() }

// UI is the Swagger page mounted at /api/doc/.
func UI() http.Handler {
	return v5emb.NewHandlerWithConfig(swgui.Config{
		Title:       "mediamtx-dynamic",
		SwaggerJSON: "/openapi.json",
		BasePath:    "/api/doc/",
	})
}

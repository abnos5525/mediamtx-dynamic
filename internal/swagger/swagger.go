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
	URL        string `json:"url" example:"udp://224.2.0.1:5493/restream/bbcdcadb-e98e-4650-8eb2-10261f7c9ad0_720"`
	OutputType int    `json:"output_type" example:"6"`
	Format     string `json:"format" example:"rtp"`
}

type streamURL struct {
	URL        string `json:"url"`
	OutputType int    `json:"output_type"`
	ID         string `json:"id"`
	HLS        string `json:"hls"`
	WebRTC     string `json:"webrtc"`
	RTSP       string `json:"rtsp"`
}

type streamStop struct {
	URL string `json:"url" example:"udp://224.2.0.1:5493/restream/bbcdcadb-e98e-4650-8eb2-10261f7c9ad0_720"`
	ID  string `json:"id,omitempty"`
}

type streamMsg struct {
	Message string `json:"message"`
}

type streamID struct {
	ID string `path:"id"`
}

type streamMeta struct {
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

type streamBox struct {
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	W          float64 `json:"w"`
	H          float64 `json:"h"`
	Label      string  `json:"label"`
	Confidence float64 `json:"confidence,omitempty"`
}

type streamOverlay struct {
	ID        string      `json:"id"`
	Timestamp int64       `json:"timestamp"`
	Width     int         `json:"width,omitempty"`
	Height    int         `json:"height,omitempty"`
	Boxes     []streamBox `json:"boxes"`
}

func Spec() []byte {
	r := openapi3.NewReflector()
	r.SpecEns().Info.WithTitle("mediamtx-dynamic").WithVersion("1.0.0").
		WithDescription("MediaMTX video + AI overlay sniff. output_type 6=LL-HLS, 7=WHEP.")

	must := func(op openapi.OperationContext, err error) openapi.OperationContext {
		if err != nil {
			panic(err)
		}
		return op
	}
	add := func(op openapi.OperationContext) {
		if err := r.AddOperation(op); err != nil {
			panic(err)
		}
	}

	st := must(r.NewOperationContext(http.MethodPost, "/stream/start"))
	st.SetSummary("Start MediaMTX path + AI sniff; 6=HLS 7=WHEP")
	st.AddReqStructure(new(streamStart))
	st.AddRespStructure(new(streamURL), func(cu *openapi.ContentUnit) { cu.HTTPStatus = http.StatusOK })
	add(st)

	sp := must(r.NewOperationContext(http.MethodPost, "/stream/stop"))
	sp.SetSummary("Stop path and AI sniff")
	sp.AddReqStructure(new(streamStop))
	sp.AddRespStructure(new(streamMsg), func(cu *openapi.ContentUnit) { cu.HTTPStatus = http.StatusOK })
	add(sp)

	meta := must(r.NewOperationContext(http.MethodGet, "/stream/meta"))
	meta.SetSummary("List active streams")
	meta.AddRespStructure(new([]streamMeta), func(cu *openapi.ContentUnit) { cu.HTTPStatus = http.StatusOK })
	add(meta)

	one := must(r.NewOperationContext(http.MethodGet, "/stream/meta/{id}"))
	one.SetSummary("One stream status")
	one.AddReqStructure(new(streamID))
	one.AddRespStructure(new(streamMeta), func(cu *openapi.ContentUnit) { cu.HTTPStatus = http.StatusOK })
	add(one)

	ov := must(r.NewOperationContext(http.MethodGet, "/stream/overlay/{id}"))
	ov.SetSummary("Delay-compensated AI boxes (query lag_ms)")
	ov.AddReqStructure(new(streamID))
	ov.AddRespStructure(new(streamOverlay), func(cu *openapi.ContentUnit) { cu.HTTPStatus = http.StatusOK })
	add(ov)

	b, err := json.Marshal(r.Spec)
	if err != nil {
		panic(err)
	}
	return b
}

func Register(mux *http.ServeMux) {
	doc := Spec()
	mux.HandleFunc("GET /openapi.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(doc)
	})
	mux.HandleFunc("GET /api/doc", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/api/doc/", http.StatusFound)
	})
	ui := v5emb.NewHandlerWithConfig(swgui.Config{
		Title:       "mediamtx-dynamic",
		SwaggerJSON: "/openapi.json",
		BasePath:    "/api/doc/",
	})
	mux.Handle("/api/doc/", ui)
}

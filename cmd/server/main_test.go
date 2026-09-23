package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseIngest(t *testing.T) {
	in, err := parseIngest("udp://224.2.0.1:5493/restream/cam_720")
	if err != nil {
		t.Fatal(err)
	}
	if in.rtsp || in.host != "224.2.0.1" || in.port != "5493" || !strings.HasPrefix(in.name, "s") {
		t.Fatalf("%+v", in)
	}
	rtsp, err := parseIngest("rtsp://cam.local/live")
	if err != nil || !rtsp.rtsp {
		t.Fatal(err, rtsp)
	}
	if _, err := parseIngest("http://example.com/a"); err == nil {
		t.Fatal("http must be rejected")
	}
	if _, err := parseIngest("udp://not-an-ip:1"); err == nil {
		t.Fatal("hostname udp must be rejected")
	}
}

func TestWriteStartWebRTCOmitsHLS(t *testing.T) {
	rec := httptest.NewRecorder()
	writeStart(rec, "s57481d6abfb5719f", 7)
	var got map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["hls"]; ok {
		t.Fatalf("hls must be absent: %v", got)
	}
	url, _ := got["url"].(string)
	if got["output_type"] != float64(7) || !strings.HasSuffix(url, "/s57481d6abfb5719f/whep") {
		t.Fatalf("%v", got)
	}
}

func TestStartRejectsBadType(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/stream/start", strings.NewReader(`{"url":"udp://224.2.0.1:5484","output_type":1}`))
	rec := httptest.NewRecorder()
	handleStart(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatal(rec.Code, rec.Body.String())
	}
}

package mtx

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPathBodyRTP(t *testing.T) {
	b, err := PathBody("224.2.0.1", "5493", "rtp")
	if err != nil {
		t.Fatal(err)
	}
	if b["source"] != "udp+rtp://224.2.0.1:5493" {
		t.Fatalf("%v", b["source"])
	}
	if b["rtpSDP"] == nil || b["rtpSDP"] == "" {
		t.Fatal("missing sdp")
	}
}

func TestUpsertAdd(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v3/config/paths/add/testpath" {
			t.Fatalf("path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c := New(srv.URL)
	if err := c.Upsert("testpath", map[string]any{"source": "udp+mpegts://1.2.3.4:5"}); err != nil {
		t.Fatal(err)
	}
}

func TestListPaths(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{{"name": "a", "ready": true}},
		})
	}))
	defer srv.Close()
	c := New(srv.URL)
	items, err := c.ListPaths()
	if err != nil || len(items) != 1 || !items[0].Ready {
		t.Fatalf("%v %v", items, err)
	}
}

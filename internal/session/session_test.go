package session

import "testing"

func TestParseIngest(t *testing.T) {
	host, port, name, err := ParseIngest("udp://224.2.0.1:5493/restream/bbcdcadb-e98e-4650-8eb2-10261f7c9ad0_720")
	if err != nil {
		t.Fatal(err)
	}
	if host != "224.2.0.1" || port != "5493" {
		t.Fatalf("host/port %s %s", host, port)
	}
	if name != "restream_bbcdcadb-e98e-4650-8eb2-10261f7c9ad0_720" {
		t.Fatalf("name %q", name)
	}
}

func TestParseIngestBad(t *testing.T) {
	if _, _, _, err := ParseIngest("http://x"); err == nil {
		t.Fatal("want error")
	}
}

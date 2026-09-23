package swagger

import (
	"strings"
	"testing"
)

func TestSpec(t *testing.T) {
	b := string(spec())
	if len(b) < 100 {
		t.Fatal("empty spec")
	}
	for _, want := range []string{"/stream/start", "/stream/stop", "/health", "output_type"} {
		if !strings.Contains(b, want) {
			t.Fatalf("spec missing %s", want)
		}
	}
}

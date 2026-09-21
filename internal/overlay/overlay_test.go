package overlay

import (
	"testing"

	"github.com/abnos5525/mediamtx-dynamic/internal/ai"
)

func TestStoreLag(t *testing.T) {
	s := NewStore()
	for i := 0; i < 5; i++ {
		f := &ai.AIFrame{}
		f.Frame.Timestamp = int64(1000 * (i + 1))
		f.Results = []ai.AIResult{{Objects: []ai.AIObject{{
			Name: "p", BBox: ai.BBox{X: float64(i), Y: 1, Width: 2, Height: 3},
		}}}}
		s.Push("cam", f)
	}
	v, ok := s.Get("cam", 2000)
	if !ok {
		t.Fatal("missing")
	}
	if v.Timestamp != 3000 {
		t.Fatalf("got ts %d want ~3000", v.Timestamp)
	}
	if len(v.Boxes) != 1 || v.Boxes[0].X != 2 {
		t.Fatalf("boxes %#v", v.Boxes)
	}
}

func TestDropBlank(t *testing.T) {
	s := NewStore()
	hit := &ai.AIFrame{}
	hit.Frame.Timestamp = 1000
	hit.Results = []ai.AIResult{{Objects: []ai.AIObject{{Name: "a", BBox: ai.BBox{Width: 1, Height: 1}}}}}
	s.Push("x", hit)
	blank := &ai.AIFrame{}
	blank.Frame.Timestamp = 1500
	blank.Results = []ai.AIResult{{Objects: nil}}
	s.Push("x", blank)
	v, _ := s.Get("x", 0)
	if len(v.Boxes) != 1 {
		t.Fatalf("blank should not wipe hit: %#v", v)
	}
}

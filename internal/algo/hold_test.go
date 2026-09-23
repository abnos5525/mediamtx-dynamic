package algo

import (
	"testing"

	"github.com/abnos5525/mediamtx-dynamic/internal/ai"
)

func TestHold(t *testing.T) {
	frames := make([]*ai.AIFrame, 3)
	arr := make([]int64, 3)
	for i := 0; i < 3; i++ {
		f := &ai.AIFrame{}
		f.Frame.Timestamp = int64((i + 1) * 1000)
		f.Results = []ai.AIResult{{Objects: []ai.AIObject{{Name: "x"}}}}
		frames[i] = f
		arr[i] = int64(1000000 + i*1000)
	}
	got := Hold(frames, arr, 1500, 1003000, 0)
	if got == nil || got.Frame.Timestamp != 1000 {
		t.Fatalf("got %#v", got)
	}
}

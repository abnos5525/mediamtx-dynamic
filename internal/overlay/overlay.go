package overlay

import (
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/abnos5525/mediamtx-dynamic/internal/ai"
	"github.com/abnos5525/mediamtx-dynamic/internal/algo"
)

type Box struct {
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	W          float64 `json:"w"`
	H          float64 `json:"h"`
	Label      string  `json:"label"`
	Confidence float64 `json:"confidence,omitempty"`
	TrackID    int     `json:"track_id,omitempty"`
}

type Frame struct {
	Timestamp int64 `json:"timestamp"`
	Boxes     []Box `json:"boxes"`
}

type View struct {
	ID        string  `json:"id"`
	Timestamp int64   `json:"timestamp"`
	Width     int     `json:"width,omitempty"`
	Height    int     `json:"height,omitempty"`
	Boxes     []Box   `json:"boxes"`
	Frames    []Frame `json:"frames,omitempty"`
	OffsetMS  int64   `json:"offset_ms,omitempty"`
}

type Store struct {
	mu   sync.Mutex
	bufs map[string]*buf
}

type buf struct {
	frames []*ai.AIFrame
	arr    []int64
	w, h   int
}

func NewStore() *Store {
	return &Store{bufs: map[string]*buf{}}
}

func (s *Store) Push(id string, f *ai.AIFrame) {
	if f == nil {
		return
	}
	now := time.Now().UnixMilli()
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.bufs[id]
	if b == nil {
		b = &buf{}
		s.bufs[id] = b
	}
	if dropBlank(b, f) {
		return
	}
	pushAI(b, f, now)
}

func (s *Store) SetSize(id string, w, h int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.bufs[id]
	if b == nil {
		b = &buf{}
		s.bufs[id] = b
	}
	if w > 0 {
		b.w = w
	}
	if h > 0 {
		b.h = h
	}
}

func (s *Store) Clear(id string) {
	s.mu.Lock()
	delete(s.bufs, id)
	s.mu.Unlock()
}

func (s *Store) Get(id string, lagMS int64) (View, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.bufs[id]
	if b == nil {
		return View{}, false
	}
	if lagMS < 0 {
		lagMS = 600
	}
	now := time.Now().UnixMilli()
	picked := algo.Hold(b.frames, b.arr, lagMS, now, aiOffsetMS())
	v := fromAI(id, picked)
	v.Width, v.Height = b.w, b.h
	v.Frames = frameWindow(b.frames, 4000, 32)
	v.OffsetMS = aiOffsetMS()
	return v, true
}

func (s *Store) MetaAI(id string) (ageMS int64, fps float64, has bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.bufs[id]
	if b == nil || len(b.arr) == 0 {
		return 0, 0, false
	}
	now := time.Now().UnixMilli()
	ageMS = now - b.arr[len(b.arr)-1]
	if len(b.arr) >= 2 {
		span := float64(b.arr[len(b.arr)-1] - b.arr[0])
		if span > 0 {
			fps = float64(len(b.arr)-1) * 1000 / span
		}
	}
	return ageMS, fps, true
}

func aiOffsetMS() int64 {
	v, _ := strconv.ParseInt(os.Getenv("AI_OFFSET_MS"), 10, 64)
	if v < -10000 || v > 10000 {
		return 0
	}
	return v
}

func aiHits(f *ai.AIFrame) bool {
	if f == nil {
		return false
	}
	for _, r := range f.Results {
		if len(r.Objects) > 0 {
			return true
		}
	}
	return false
}

func dropBlank(b *buf, f *ai.AIFrame) bool {
	n := len(b.frames)
	if n == 0 || aiHits(f) {
		return false
	}
	last := b.frames[n-1]
	if !aiHits(last) {
		return false
	}
	if len(f.Results) == 0 {
		return true
	}
	dt := f.Frame.Timestamp - last.Frame.Timestamp
	if dt < 0 {
		dt = -dt
	}
	return dt < 2500
}

func pushAI(b *buf, f *ai.AIFrame, arrived int64) {
	ts := f.Frame.Timestamp
	n := len(b.frames)
	switch {
	case n > 0 && b.frames[n-1].Frame.Timestamp == ts:
		if !aiHits(f) && aiHits(b.frames[n-1]) {
			b.arr[n-1] = arrived
			return
		}
		b.frames[n-1] = f
		b.arr[n-1] = arrived
	case n == 0 || b.frames[n-1].Frame.Timestamp < ts:
		b.frames = append(b.frames, f)
		b.arr = append(b.arr, arrived)
	default:
		i := 0
		for i < n && b.frames[i].Frame.Timestamp < ts {
			i++
		}
		if i < n && b.frames[i].Frame.Timestamp == ts {
			if !aiHits(f) && aiHits(b.frames[i]) {
				b.arr[i] = arrived
			} else {
				b.frames[i] = f
				b.arr[i] = arrived
			}
		} else {
			b.frames = append(b.frames, nil)
			copy(b.frames[i+1:], b.frames[i:])
			b.frames[i] = f
			b.arr = append(b.arr, 0)
			copy(b.arr[i+1:], b.arr[i:])
			b.arr[i] = arrived
		}
	}
	latest := b.frames[len(b.frames)-1].Frame.Timestamp
	cut := latest - 12000
	i := 0
	for i < len(b.frames) && b.frames[i].Frame.Timestamp < cut {
		i++
	}
	b.frames = b.frames[i:]
	b.arr = b.arr[i:]
	if n := len(b.frames); n > 400 {
		b.frames = b.frames[n-400:]
		b.arr = b.arr[n-400:]
	}
}

func fromAI(id string, f *ai.AIFrame) View {
	v := View{ID: id, Boxes: []Box{}}
	if f == nil {
		return v
	}
	v.Timestamp = f.Frame.Timestamp
	for _, r := range f.Results {
		for _, obj := range r.Objects {
			label := obj.Name
			if label == "" {
				label = r.Model
			}
			v.Boxes = append(v.Boxes, Box{
				X: obj.BBox.X, Y: obj.BBox.Y, W: obj.BBox.Width, H: obj.BBox.Height,
				Label: label, Confidence: obj.Confidence, TrackID: obj.TrackID,
			})
		}
	}
	return v
}

func frameWindow(frames []*ai.AIFrame, spanMS int64, nMax int) []Frame {
	n := len(frames)
	if n == 0 {
		return nil
	}
	start := 0
	if latest := frames[n-1].Frame.Timestamp; latest > 0 && spanMS > 0 {
		cut := latest - spanMS
		for start < n && frames[start].Frame.Timestamp < cut {
			start++
		}
	}
	buf := frames[start:]
	if nMax > 1 && len(buf) > nMax {
		step := len(buf) - 1
		picked := make([]*ai.AIFrame, nMax)
		for i := 0; i < nMax; i++ {
			picked[i] = buf[i*step/(nMax-1)]
		}
		buf = picked
	}
	out := make([]Frame, len(buf))
	for i, f := range buf {
		v := fromAI("", f)
		out[i] = Frame{Timestamp: v.Timestamp, Boxes: v.Boxes}
	}
	return out
}

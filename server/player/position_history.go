package player

import (
	"time"

	"github.com/go-gl/mathgl/mgl64"
)

// positionClock stamps position history. Tests replace it.
var positionClock = time.Now

// positionHistorySize covers two seconds of movement at one sample per tick,
// longer than the widest placement view window.
const positionHistorySize = 40

type positionSample struct {
	at  time.Time
	pos mgl64.Vec3
}

// positionHistory keeps the most recent positions of a player, each stamped
// with the time the server applied it. A player stays at a sample's position
// until the next sample.
type positionHistory struct {
	samples [positionHistorySize]positionSample
	next, n int
}

func (h *positionHistory) add(at time.Time, pos mgl64.Vec3) {
	h.samples[h.next] = positionSample{at: at, pos: pos}
	h.next = (h.next + 1) % positionHistorySize
	h.n = min(h.n+1, positionHistorySize)
}

func (h *positionHistory) reset() {
	h.n, h.next = 0, 0
}

// anyClearSince reports whether clear holds for a position the player occupied
// at any time from since onward. The sample in effect at since counts. When the
// history does not reach back to since, the unknown earlier positions do not.
func (h *positionHistory) anyClearSince(since time.Time, clear func(mgl64.Vec3) bool) bool {
	for i := range h.n {
		sample := h.samples[(h.next-1-i+positionHistorySize)%positionHistorySize]
		if clear(sample.pos) {
			return true
		}
		if !sample.at.After(since) {
			return false
		}
	}
	return false
}

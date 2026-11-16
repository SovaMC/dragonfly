package player

import (
	"testing"
	"time"

	"github.com/go-gl/mathgl/mgl64"
)

func TestPositionHistoryAnyClearSince(t *testing.T) {
	start := time.Unix(1_000_000, 0)
	clearAtZero := func(pos mgl64.Vec3) bool { return pos.X() == 0 }
	tick := func(i int) time.Time { return start.Add(time.Duration(i) * 50 * time.Millisecond) }

	for _, test := range []struct {
		name  string
		fill  func(h *positionHistory)
		since time.Time
		want  bool
	}{
		{
			name:  "empty",
			fill:  func(*positionHistory) {},
			since: start,
		},
		{
			// The sample at tick 0 is still in effect at tick 1.
			name: "sample_in_effect_at_since_counts",
			fill: func(h *positionHistory) {
				h.add(tick(0), mgl64.Vec3{0, 0, 0})
				h.add(tick(4), mgl64.Vec3{1, 0, 0})
			},
			since: tick(1),
			want:  true,
		},
		{
			name: "older_samples_do_not_count",
			fill: func(h *positionHistory) {
				h.add(tick(0), mgl64.Vec3{0, 0, 0})
				h.add(tick(2), mgl64.Vec3{1, 0, 0})
				h.add(tick(4), mgl64.Vec3{1, 0, 0})
			},
			since: tick(3),
		},
		{
			// Wrapping drops the clear sample, so the history no longer reaches
			// back to since and the unknown earlier position is not trusted.
			name: "overwritten_sample_does_not_count",
			fill: func(h *positionHistory) {
				h.add(tick(0), mgl64.Vec3{0, 0, 0})
				for i := 1; i <= positionHistorySize; i++ {
					h.add(tick(i), mgl64.Vec3{1, 0, 0})
				}
			},
			since: tick(0),
		},
		{
			name: "reset_forgets_everything",
			fill: func(h *positionHistory) {
				h.add(tick(0), mgl64.Vec3{0, 0, 0})
				h.reset()
				h.add(tick(4), mgl64.Vec3{1, 0, 0})
			},
			since: tick(1),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var h positionHistory
			test.fill(&h)
			if got := h.anyClearSince(test.since, clearAtZero); got != test.want {
				t.Fatalf("anyClearSince = %v, want %v", got, test.want)
			}
		})
	}
}

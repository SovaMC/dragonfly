package entity

import (
	"testing"

	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl64"
)

func TestProjectileTrajectoryAppliesDragBeforeGravity(t *testing.T) {
	const gravity, drag = 0.065, 0.0085
	w := world.Config{Synchronous: true}.New()
	t.Cleanup(func() { _ = w.Close() })

	start := mgl64.Vec3{0.5, 200, 0.5}
	want, vel := start, mgl64.Vec3{0, 0.6, 2.35}
	conf := ProjectileBehaviourConfig{Gravity: gravity, Drag: drag}
	handle := world.EntitySpawnOpts{Position: start, Velocity: vel}.New(ArrowType, conf)
	mustDo(t, w, func(tx *world.Tx) {
		e := tx.AddEntity(handle)
		for i := 0; i < 40; i++ {
			e.(world.TickerEntity).Tick(tx, int64(i))
			vel = mgl64.Vec3{vel[0] * (1 - drag), vel[1]*(1-drag) - gravity, vel[2] * (1 - drag)}
			want = want.Add(vel)
			if !e.Position().ApproxEqualThreshold(want, 1e-9) {
				t.Fatalf("tick %d: position %v, want %v", i, e.Position(), want)
			}
		}
	})
}

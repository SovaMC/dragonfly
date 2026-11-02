package entity_test

import (
	"context"
	"testing"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/block/cube/trace"
	"github.com/df-mc/dragonfly/server/entity"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl64"
)

func TestProjectileIgnoreBlockPassesOnce(t *testing.T) {
	world.DefaultBlockRegistry.Finalize()
	w := world.Config{Synchronous: true}.New()
	t.Cleanup(func() { _ = w.Close() })

	var hit trace.Result
	var ignored []cube.Pos
	conf := entity.ProjectileBehaviourConfig{
		Hit: func(_ *entity.Ent, _ *world.Tx, target trace.Result) { hit = target },
		IgnoreBlock: func(_ *entity.Ent, _ *world.Tx, pos cube.Pos, b world.Block) bool {
			if _, ok := b.(block.Stone); !ok || len(ignored) > 0 {
				return false
			}
			ignored = append(ignored, pos)
			return true
		},
	}
	handle := world.EntitySpawnOpts{Position: mgl64.Vec3{0.5, 64.5, 0.5}, Velocity: mgl64.Vec3{4}}.New(entity.ArrowType, conf)
	first, second := cube.Pos{3, 64, 0}, cube.Pos{6, 64, 0}
	err := w.Do(func(tx *world.Tx) {
		tx.SetBlock(first, block.Stone{}, nil)
		tx.SetBlock(second, block.Stone{}, nil)
		e := tx.AddEntity(handle)
		for i := 0; i < 10 && hit == nil; i++ {
			e.(world.TickerEntity).Tick(tx, int64(i))
		}
	}).Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(ignored) != 1 || ignored[0] != first {
		t.Fatalf("ignored = %v, want only %v", ignored, first)
	}
	r, ok := hit.(trace.BlockResult)
	if !ok || r.BlockPosition() != second {
		t.Fatalf("hit = %#v, want the block at %v", hit, second)
	}
}

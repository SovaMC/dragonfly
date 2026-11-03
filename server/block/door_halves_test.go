package block_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/block/model"
	"github.com/df-mc/dragonfly/server/world"
)

func TestDoorHalvesShareOpenAndHinge(t *testing.T) {
	world.DefaultBlockRegistry.Finalize()
	w := world.Config{Synchronous: true}.New()
	t.Cleanup(func() { _ = w.Close() })

	want := model.Door{Facing: cube.South, Open: true, Right: true}.BBox(cube.Pos{}, nil)
	bottom, top := cube.Pos{0, 64, 0}, cube.Pos{0, 65, 0}
	err := w.Do(func(tx *world.Tx) {
		tx.SetBlock(bottom, block.WoodDoor{Wood: block.DarkOakWood(), Facing: cube.South, Open: true}, nil)
		tx.SetBlock(top, block.WoodDoor{Wood: block.DarkOakWood(), Facing: cube.South, Top: true, Right: true}, nil)

		for _, pos := range []cube.Pos{bottom, top} {
			if got := tx.Block(pos).Model().BBox(pos, tx); !reflect.DeepEqual(got, want) {
				t.Errorf("box at %v = %v, want %v", pos, got, want)
			}
		}
	}).Wait(context.Background())
	if err != nil {
		t.Fatal(err)
	}
}

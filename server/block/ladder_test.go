package block

import (
	"testing"

	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl64"
)

func TestLadderSupportsEveryFullCube(t *testing.T) {
	w := world.Config{Dim: world.End, Synchronous: true}.New()
	defer w.Close()
	loader := world.NewLoader(1, w, world.NopViewer{})
	defer runWorld(w, func(tx *world.Tx) { loader.Close(tx) })

	pos := cube.Pos{0, 64, 0}
	for _, test := range []struct {
		name  string
		block world.Block
		want  bool
	}{
		{name: "stone", block: Stone{}, want: true},
		{name: "leaves", block: Leaves{Type: OakLeaves()}, want: true},
		{name: "glass", block: Glass{}, want: true},
		{name: "dirt path", block: DirtPath{}, want: true},
		{name: "farmland", block: Farmland{}, want: true},
		{name: "soul sand", block: SoulSand{}, want: true},
		{name: "air", block: Air{}, want: false},
		{name: "slab", block: Slab{Block: Stone{}}, want: false},
		{name: "stairs", block: Stairs{Block: Stone{}}, want: false},
		{name: "fence", block: WoodFence{Wood: OakWood()}, want: false},
		{name: "carpet", block: Carpet{}, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			runWorld(w, func(tx *world.Tx) {
				loader.Move(tx, mgl64.Vec3{0, 64, 0})
				loader.Load(tx, 4)
				tx.SetBlock(pos, test.block, nil)
				if got := ladderSupport(pos, tx); got != test.want {
					t.Fatalf("ladderSupport(%s) = %v, want %v", test.name, got, test.want)
				}
			})
		})
	}
}

func TestLadderStaysOnLeavesAfterANeighbourUpdate(t *testing.T) {
	w := world.Config{Dim: world.End, Synchronous: true}.New()
	defer w.Close()
	loader := world.NewLoader(1, w, world.NopViewer{})
	defer runWorld(w, func(tx *world.Tx) { loader.Close(tx) })

	support, ladderPos := cube.Pos{0, 64, 0}, cube.Pos{0, 64, 1}
	runWorld(w, func(tx *world.Tx) {
		loader.Move(tx, mgl64.Vec3{0, 64, 0})
		loader.Load(tx, 4)
		tx.SetBlock(support, Leaves{Type: OakLeaves(), Persistent: true}, nil)
		ladder := Ladder{Facing: cube.South}
		tx.SetBlock(ladderPos, ladder, nil)
		ladder.NeighbourUpdateTick(ladderPos, support, tx)
		if _, ok := tx.Block(ladderPos).(Ladder); !ok {
			t.Fatalf("ladder on leaves broke after a neighbour update")
		}
		tx.SetBlock(support, Air{}, nil)
		ladder.NeighbourUpdateTick(ladderPos, support, tx)
		if _, ok := tx.Block(ladderPos).(Ladder); ok {
			t.Fatalf("ladder kept hanging on air")
		}
	})
}

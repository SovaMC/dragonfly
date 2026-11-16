package block

import (
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/block/model"
	"github.com/df-mc/dragonfly/server/item"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl64"
	"time"
)

// Ladder is a wooden block used for climbing walls either vertically or horizontally. They can be placed only on
// the sides of other blocks.
type Ladder struct {
	transparent
	sourceWaterDisplacer

	// Facing is the side of the block the ladder is currently attached to.
	Facing cube.Direction
}

// NeighbourUpdateTick ...
func (l Ladder) NeighbourUpdateTick(pos, _ cube.Pos, tx *world.Tx) {
	if !ladderSupport(pos.Side(l.Facing.Face().Opposite()), tx) {
		breakBlock(l, pos, tx)
	}
}

// UseOnBlock ...
func (l Ladder) UseOnBlock(pos cube.Pos, face cube.Face, _ mgl64.Vec3, tx *world.Tx, user item.User, ctx *item.UseContext) bool {
	pos, face, used := firstReplaceable(tx, pos, face, l)
	if !used {
		return false
	}
	if face == cube.FaceUp || face == cube.FaceDown {
		return false
	}
	if !ladderSupport(pos.Side(face.Opposite()), tx) {
		found := false
		for _, i := range []cube.Face{cube.FaceSouth, cube.FaceNorth, cube.FaceEast, cube.FaceWest} {
			if ladderSupport(pos.Side(i), tx) {
				found = true
				face = i.Opposite()
				break
			}
		}
		if !found {
			return false
		}
	}
	l.Facing = face.Direction()

	place(tx, pos, l, user, ctx)
	return placed(ctx)
}

// ladderSupport reports whether a ladder may hang on the block at pos. Opaque blocks qualify, and so does any
// block whose single collision box fills the block's footprint, such as leaves, glass, dirt paths, farmland and
// soul sand. Slabs, stairs, fences and other partial shapes do not.
func ladderSupport(pos cube.Pos, tx *world.Tx) bool {
	b := tx.Block(pos)
	if diffuser, ok := b.(LightDiffuser); !ok || diffuser.LightDiffusionLevel() == 15 {
		return true
	}
	boxes := b.Model().BBox(pos, tx)
	if len(boxes) != 1 {
		return false
	}
	const epsilon = 1e-6
	minimum, maximum := boxes[0].Min(), boxes[0].Max()
	return minimum.X() <= epsilon && minimum.Y() <= epsilon && minimum.Z() <= epsilon &&
		maximum.X() >= 1-epsilon && maximum.Z() >= 1-epsilon && maximum.Y() >= 0.875-epsilon
}

// EntityInside ...
func (l Ladder) EntityInside(_ cube.Pos, _ *world.Tx, e world.Entity) {
	if fallEntity, ok := e.(fallDistanceEntity); ok {
		fallEntity.ResetFallDistance()
	}
}

// SideClosed ...
func (l Ladder) SideClosed(cube.Pos, cube.Pos, *world.Tx) bool {
	return false
}

// BreakInfo ...
func (l Ladder) BreakInfo() BreakInfo {
	return newBreakInfo(0.4, alwaysHarvestable, axeEffective, oneOf(l))
}

// FuelInfo ...
func (Ladder) FuelInfo() item.FuelInfo {
	return newFuelInfo(time.Second * 15)
}

// EncodeItem ...
func (l Ladder) EncodeItem() (name string, meta int16) {
	return "minecraft:ladder", 0
}

// EncodeBlock ...
func (l Ladder) EncodeBlock() (string, map[string]any) {
	if l.Facing == unknownDirection {
		return "minecraft:ladder", map[string]any{"facing_direction": int32(0)}
	}
	return "minecraft:ladder", map[string]any{"facing_direction": int32(l.Facing + 2)}
}

// Model ...
func (l Ladder) Model() world.BlockModel {
	return model.Ladder{Facing: l.Facing}
}

// allLadders ...
func allLadders() (b []world.Block) {
	for _, f := range append(cube.Directions(), unknownDirection) {
		b = append(b, Ladder{Facing: f})
	}
	return
}

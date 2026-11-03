package model

import (
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
)

// Solid is the model of a fully solid block. Blocks with this model, such as stone or wooden planks, have a
// 1x1x1 collision box.
type Solid struct{}

// full is a BBox that occupies a full block.
var full = cube.Box(0, 0, 0, 1, 1, 1)

// fullBoxes is the shared result of Solid.BBox. Its capacity is limited so that appending to it makes a copy.
var fullBoxes = []cube.BBox{full}[:1:1]

// BBox returns a physics.BBox spanning a full block.
func (Solid) BBox(cube.Pos, world.BlockSource) []cube.BBox {
	return fullBoxes
}

// FaceSolid always returns true.
func (Solid) FaceSolid(cube.Pos, cube.Face, world.BlockSource) bool {
	return true
}

package model

import (
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/world"
)

// Door is a model used for doors. It has no solid faces and a bounding box that changes depending on
// the direction of the door, whether it is open, and the side of its hinge.
type Door struct {
	// Facing is the direction that the door is facing when closed.
	Facing cube.Direction
	// Open specifies if the Door is open. The direction it opens towards depends on the Right field.
	Open bool
	// Right specifies the attachment side of the door and, with that, the direction it opens in.
	Right bool
	// Top specifies if this is the top half of the door. The client reads whether a door is open from its bottom half
	// and the side of the hinge from its top half, so the other half of the door decides these values.
	Top bool
}

// BBox returns a physics.BBox that depends on if the Door is open, what direction it is facing and whether it is
// attached to the right/left side of a block.
func (d Door) BBox(pos cube.Pos, src world.BlockSource) []cube.BBox {
	if src != nil {
		if d.Top {
			if bottom, ok := src.Block(pos.Side(cube.FaceDown)).Model().(Door); ok {
				d.Open = bottom.Open
			}
		} else if top, ok := src.Block(pos.Side(cube.FaceUp)).Model().(Door); ok {
			d.Right = top.Right
		}
	}
	if d.Open {
		if d.Right {
			return []cube.BBox{full.ExtendTowards(d.Facing.RotateLeft().Face(), -0.8125)}
		}
		return []cube.BBox{full.ExtendTowards(d.Facing.RotateRight().Face(), -0.8125)}
	}
	return []cube.BBox{full.ExtendTowards(d.Facing.Face(), -0.8125)}
}

// FaceSolid always returns false.
func (d Door) FaceSolid(cube.Pos, cube.Face, world.BlockSource) bool {
	return false
}

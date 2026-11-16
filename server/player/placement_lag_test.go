package player

import (
	"testing"
	"time"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/item"
	"github.com/df-mc/dragonfly/server/session"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/go-gl/mathgl/mgl64"
	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// latencyConn reports a fixed one-way latency, as Session.Latency does for a
// real connection.
type latencyConn struct {
	swimmingConn
	latency time.Duration
}

func (c *latencyConn) Latency() time.Duration { return c.latency }

// setPositionClock replaces the clock that stamps position history for one test.
func setPositionClock(t *testing.T, now *time.Time) {
	t.Helper()
	previous := positionClock
	positionClock = func() time.Time { return *now }
	t.Cleanup(func() { positionClock = previous })
}

// sessionPlayer adds a player backed by an in-memory session with the given
// one-way latency. Defer the returned function inside the same transaction.
func sessionPlayer(tx *world.Tx, pos mgl64.Vec3, latency time.Duration) (*Player, *session.Session, func()) {
	conn := &latencyConn{swimmingConn: swimmingConn{packets: make(chan packet.Packet, 4096)}, latency: latency}
	s := (session.Config{MaxChunkRadius: 1, HandleStop: func(*world.Tx, session.Controllable) {}}).New(conn)
	handle := world.EntitySpawnOpts{ID: uuid.New()}.New(Type, Config{Session: s, Position: pos})
	p := tx.AddEntity(handle).(*Player)
	s.SetHandle(handle, p.Skin())
	return p, s, func() {
		s.Close(nil, p)
		_ = tx.RemoveEntity(p).Close()
		s.CloseConnection()
	}
}

func TestPlacementAllowsOtherPlayerThePlacerCouldNotYetSee(t *testing.T) {
	// The block goes to {2, 64, 0}. The other player steps from x=3.5, clear of
	// it, to x=2.9, where their box overlaps it. The placer sees that step one
	// round trip plus client interpolation after the server applied it.
	const (
		clearX   = 3.5
		overlapX = 2.9
	)
	for _, test := range []struct {
		name      string
		latency   time.Duration // one way
		movedAgo  time.Duration
		targetY   int
		wantPlace bool
	}{
		{name: "step_within_round_trip", latency: 75 * time.Millisecond, movedAgo: 100 * time.Millisecond, targetY: 64, wantPlace: true},
		{name: "high_ping_widens_window", latency: 150 * time.Millisecond, movedAgo: 400 * time.Millisecond, targetY: 64, wantPlace: true},
		{name: "zero_ping_old_step", movedAgo: 400 * time.Millisecond, targetY: 64},
		{name: "standing_in_place", latency: 75 * time.Millisecond, movedAgo: 2 * time.Second, targetY: 64},
		{name: "block_at_their_head", latency: 75 * time.Millisecond, movedAgo: 100 * time.Millisecond, targetY: 65},
	} {
		t.Run(test.name, func(t *testing.T) {
			now := time.Unix(1_000_000, 0)
			setPositionClock(t, &now)
			w := world.Config{Synchronous: true}.New()
			t.Cleanup(func() { _ = w.Close() })
			if err := w.Do(func(tx *world.Tx) {
				for x := -1; x <= 5; x++ {
					for z := -1; z <= 1; z++ {
						tx.SetBlock(cube.Pos{x, 63, z}, block.Stone{}, nil)
					}
				}
				// A pillar beside the head-height target gives it a face to click.
				tx.SetBlock(cube.Pos{2, 64, -1}, block.Stone{}, nil)
				tx.SetBlock(cube.Pos{2, 65, -1}, block.Stone{}, nil)

				placer, _, done := sessionPlayer(tx, mgl64.Vec3{0.5, 64, 0.5}, test.latency)
				defer done()
				_ = placer.Inventory().SetItem(0, item.NewStack(block.Stone{}, 4))

				other := tx.AddEntity(world.EntitySpawnOpts{Position: mgl64.Vec3{clearX, 64, 0.5}}.New(Type, Config{
					UUID: uuid.New(), Name: "Other", Position: mgl64.Vec3{clearX, 64, 0.5},
				})).(*Player)
				stepAt := now.Add(-test.movedAgo)
				now = stepAt.Add(-time.Second)
				other.Move(mgl64.Vec3{0, 0, 0.001}, 0, 0)
				now = stepAt
				other.Move(mgl64.Vec3{overlapX - clearX, 0, 0}, 0, 0)
				now = stepAt.Add(test.movedAgo)

				clicked, face := cube.Pos{2, 63, 0}, cube.FaceUp
				if test.targetY == 65 {
					clicked, face = cube.Pos{2, 65, -1}, cube.FaceSouth
				}
				placer.UseItemOnBlock(clicked, face, mgl64.Vec3{0.5, 1, 0.5})

				target := cube.Pos{2, test.targetY, 0}
				_, placed := tx.Block(target).(block.Stone)
				if placed != test.wantPlace {
					t.Errorf("block at %v placed = %v, want %v", target, placed, test.wantPlace)
				}
			}).Wait(t.Context()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPlacementInsideSelfFollowsClientPrediction(t *testing.T) {
	// The placer stands against the wall at z=1. A ladder hung on that wall
	// occupies z 0.8125-1 of the block the placer stands in, and the server's
	// position for the placer overlaps it.
	for _, test := range []struct {
		name       string
		prediction uint8
		wantPlace  bool
	}{
		{name: "client_placed_it", prediction: protocol.ClientPredictionSuccess, wantPlace: true},
		{name: "client_refused_it", prediction: protocol.ClientPredictionFailure},
	} {
		t.Run(test.name, func(t *testing.T) {
			w := world.Config{Synchronous: true}.New()
			t.Cleanup(func() { _ = w.Close() })
			if err := w.Do(func(tx *world.Tx) {
				for x := -1; x <= 1; x++ {
					for z := -1; z <= 1; z++ {
						tx.SetBlock(cube.Pos{x, 63, z}, block.Stone{}, nil)
					}
					tx.SetBlock(cube.Pos{x, 64, 1}, block.Stone{}, nil)
				}
				placer, s, done := sessionPlayer(tx, mgl64.Vec3{0.5, 64, 0.6}, 0)
				defer done()
				_ = placer.Inventory().SetItem(0, item.NewStack(block.Ladder{}, 4))

				pk := &packet.InventoryTransaction{TransactionData: &protocol.UseItemTransactionData{
					ActionType:       protocol.UseItemActionClickBlock,
					TriggerType:      protocol.TriggerTypePlayerInput,
					BlockPosition:    protocol.BlockPos{0, 64, 1},
					BlockFace:        int32(cube.FaceNorth),
					HotBarSlot:       0,
					Position:         mgl32.Vec3{0.5, 65.62, 0.6},
					ClickedPosition:  mgl32.Vec3{0.5, 0.5, 0},
					ClientPrediction: test.prediction,
				}}
				if err := (&session.InventoryTransactionHandler{}).Handle(pk, s, tx, placer); err != nil {
					t.Fatal(err)
				}
				_, placed := tx.Block(cube.Pos{0, 64, 0}).(block.Ladder)
				if placed != test.wantPlace {
					t.Errorf("ladder placed = %v, want %v", placed, test.wantPlace)
				}
			}).Wait(t.Context()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

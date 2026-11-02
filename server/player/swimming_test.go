package player

import (
	"net"
	"testing"
	"time"

	"github.com/df-mc/dragonfly/server/block"
	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/session"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/go-gl/mathgl/mgl64"
	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/login"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func swimmingPlayer(tx *world.Tx, disabled bool) *Player {
	return tx.AddEntity(world.EntitySpawnOpts{ID: uuid.New()}.New(Type, Config{
		Position: mgl64.Vec3{0.5, 64, 0.5}, DisableSwimming: disabled,
	})).(*Player)
}

func TestSwimmingPolicyIsPerPlayerAndReversible(t *testing.T) {
	w := world.Config{Synchronous: true}.New()
	t.Cleanup(func() { _ = w.Close() })
	if err := w.Do(func(tx *world.Tx) {
		enabled, disabled := swimmingPlayer(tx, false), swimmingPlayer(tx, true)
		viewer := &itemUseStateViewer{}
		loader := world.NewLoader(1, w, viewer)
		defer loader.Close(tx)
		loader.Move(tx, enabled.Position())
		loader.Load(tx, 16)

		enabled.StartSwimming()
		if !enabled.SwimmingEnabled() || !enabled.Swimming() || Type.BBox(enabled).Height() != 0.6 || enabled.EyeHeight() != 0.4 {
			t.Fatal("default policy did not preserve the swimming pose")
		}
		disabled.StartSneaking()
		viewer.states = 0
		disabled.StartSwimming()
		if disabled.SwimmingEnabled() || disabled.Swimming() || !disabled.Sneaking() || viewer.states != 0 {
			t.Fatal("disabled swimming changed the pose or broadcast a rejected transition")
		}
		if !enabled.Swimming() {
			t.Fatal("one player's policy changed another player")
		}

		enabled.SetSwimmingEnabled(false)
		if enabled.Swimming() || Type.BBox(enabled).Height() != 1.8 || enabled.EyeHeight() != 1.62 || viewer.states != 1 {
			t.Fatal("disabling an active swim did not publish the restored standing pose")
		}
		enabled.SetSwimmingEnabled(false)
		enabled.StopSwimming()
		if viewer.states != 1 {
			t.Fatal("repeated policy/stop calls broadcast unchanged state")
		}
		enabled.SetSwimmingEnabled(true)
		if !enabled.SwimmingEnabled() || enabled.Swimming() || viewer.states != 1 {
			t.Fatal("enabling swimming started a swim without a new request")
		}
		enabled.StartSwimming()
		if !enabled.Swimming() || viewer.states != 2 {
			t.Fatal("re-enabled player could not swim again")
		}

		disabled.StopSneaking()
		tx.SetLiquid(cube.Pos{0, 64, 0}, block.Water{Depth: 8, Still: true})
		before := disabled.Position()
		disabled.Move(mgl64.Vec3{0.1, 0.1, 0.1}, 10, 0)
		if disabled.Immobile() || !disabled.Position().ApproxEqual(before.Add(mgl64.Vec3{0.1, 0.1, 0.1})) || disabled.Rotation().Yaw() != 10 {
			t.Fatal("swimming policy interfered with movement through water")
		}
	}).Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestSwimmingPolicySurvivesWorldChangesButIsNotSaved(t *testing.T) {
	source, destination := world.Config{Synchronous: true}.New(), world.Config{Synchronous: true}.New()
	t.Cleanup(func() { _ = source.Close(); _ = destination.Close() })
	var handle *world.EntityHandle
	if err := source.Do(func(tx *world.Tx) {
		p := swimmingPlayer(tx, false)
		p.SetSwimmingEnabled(false)
		if p.Data().DisableSwimming {
			t.Fatal("runtime policy leaked into persistent player data")
		}
		handle = tx.RemoveEntity(p)
	}).Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := destination.Do(func(tx *world.Tx) {
		p := tx.AddEntity(handle).(*Player)
		p.StartSwimming()
		if p.SwimmingEnabled() || p.Swimming() {
			t.Fatal("changing worlds reset the player's swimming policy")
		}
		fresh := swimmingPlayer(tx, false)
		fresh.StartSwimming()
		if !fresh.Swimming() {
			t.Fatal("a new session inherited another session's disabled swimming")
		}
	}).Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
}

// swimmingConn is an in-memory session boundary: it never listens or dials.
// The marker in drainSwimmingPackets observes the real ordered session writer.
type swimmingConn struct {
	session.Conn
	packets chan packet.Packet
}

func (*swimmingConn) IdentityData() login.IdentityData { return login.IdentityData{} }
func (*swimmingConn) RemoteAddr() net.Addr             { return &net.UDPAddr{} }
func (*swimmingConn) Proto() minecraft.Protocol        { return nil }
func (*swimmingConn) ChunkRadius() int                 { return 1 }
func (*swimmingConn) Close() error                     { return nil }
func (c *swimmingConn) WritePacket(pk packet.Packet) error {
	c.packets <- pk
	return nil
}

func drainSwimmingPackets(t *testing.T, s *session.Session, c *swimmingConn) []packet.Packet {
	t.Helper()
	marker := &packet.NetworkStackLatency{Timestamp: 123}
	s.WritePacket(marker)
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	var packets []packet.Packet
	for {
		select {
		case pk := <-c.packets:
			if pk == marker {
				return packets
			}
			packets = append(packets, pk)
		case <-timer.C:
			t.Fatal("session writer did not reach the packet barrier")
		}
	}
}

func TestSwimmingInputsCorrectTheControllingClient(t *testing.T) {
	w := world.Config{Synchronous: true}.New()
	t.Cleanup(func() { _ = w.Close() })
	if err := w.Do(func(tx *world.Tx) {
		conn := &swimmingConn{packets: make(chan packet.Packet, 4096)}
		s := (session.Config{MaxChunkRadius: 1, HandleStop: func(*world.Tx, session.Controllable) {}}).New(conn)
		defer s.CloseConnection()
		handle := world.EntitySpawnOpts{ID: uuid.New()}.New(Type, Config{
			Session: s, Position: mgl64.Vec3{0.5, 64, 0.5}, DisableSwimming: true,
		})
		p := tx.AddEntity(handle).(*Player)
		s.SetHandle(handle, p.Skin())
		defer func() { s.Close(nil, p); _ = tx.RemoveEntity(p).Close() }()
		assertPose := func(want bool) {
			t.Helper()
			packets := drainSwimmingPackets(t, s, conn)
			states := 0
			for _, pk := range packets {
				if state, ok := pk.(*packet.SetActorData); ok {
					states++
					if state.EntityRuntimeID != 1 || state.EntityMetadata.Flag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagSwimming) != want {
						t.Fatalf("wrong swimming correction: %#v", state)
					}
				}
				if _, ok := pk.(*packet.MovePlayer); ok {
					t.Fatal("swimming policy sent a movement correction")
				}
			}
			if states != 1 || p.Swimming() != want {
				t.Fatalf("swimming=%t metadata count=%d, want swimming=%t with one correction", p.Swimming(), states, want)
			}
		}
		drainSwimmingPackets(t, s, conn)
		legacy := &session.PlayerActionHandler{}
		start := &packet.PlayerAction{EntityRuntimeID: 1, ActionType: protocol.PlayerActionStartSwimming}
		if err := legacy.Handle(start, s, tx, p); err != nil {
			t.Fatal(err)
		}
		assertPose(false)
		modern := func(flag int) {
			t.Helper()
			flags := protocol.NewInputFlags(packet.InputFlagCount)
			flags.Set(flag)
			pos := p.Position()
			pk := &packet.PlayerAuthInput{InputData: flags, Position: mgl32.Vec3{float32(pos.X()), float32(pos.Y() + 1.62), float32(pos.Z())}}
			if err := (session.PlayerAuthInputHandler{}).Handle(pk, s, tx, p); err != nil {
				t.Fatal(err)
			}
		}
		modern(packet.InputFlagStartSwimming)
		assertPose(false)
		p.SetSwimmingEnabled(true)
		modern(packet.InputFlagStartSwimming)
		assertPose(true)
		p.SetSwimmingEnabled(false)
		assertPose(false)
		p.SetSwimmingEnabled(true)
		if err := legacy.Handle(start, s, tx, p); err != nil {
			t.Fatal(err)
		}
		assertPose(true)
		modern(packet.InputFlagStopSwimming)
		assertPose(false)
	}).Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
}

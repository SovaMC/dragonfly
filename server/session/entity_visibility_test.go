package session

import (
	"fmt"
	"log/slog"
	"testing"

	"github.com/df-mc/dragonfly/server/block/cube"
	"github.com/df-mc/dragonfly/server/entity/effect"
	"github.com/df-mc/dragonfly/server/item"
	"github.com/df-mc/dragonfly/server/item/inventory"
	"github.com/df-mc/dragonfly/server/player/skin"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/df-mc/dragonfly/server/world/chunk"
	"github.com/go-gl/mathgl/mgl64"
	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// Exercise the real session and chunk loader with controllable actors, including
// the retained player-list runtime IDs that ordinary AddActor fixtures lack.
type visibilityActor struct {
	Controllable
	handle *world.EntityHandle
	tx     *world.Tx
	data   *world.EntityData
}

type visibilityActorData struct {
	mode      world.GameMode
	armour    *inventory.Armour
	invisible bool
	// skin, when set, replaces the blank skin the actor otherwise reports.
	skin *skin.Skin
}

func (a *visibilityActor) H() *world.EntityHandle  { return a.handle }
func (a *visibilityActor) Tx() *world.Tx           { return a.tx }
func (a *visibilityActor) Position() mgl64.Vec3    { return a.data.Pos }
func (a *visibilityActor) Rotation() cube.Rotation { return a.data.Rot }
func (a *visibilityActor) UUID() uuid.UUID         { return a.handle.UUID() }
func (a *visibilityActor) Name() string            { return a.data.Name }
func (a *visibilityActor) NameTag() string         { return a.data.Name }
func (a *visibilityActor) Skin() skin.Skin {
	if current := a.data.Data.(*visibilityActorData).skin; current != nil {
		return *current
	}
	return skin.New(64, 64)
}
func (a *visibilityActor) GameMode() world.GameMode { return a.data.Data.(*visibilityActorData).mode }
func (a *visibilityActor) Armour() *inventory.Armour {
	return a.data.Data.(*visibilityActorData).armour
}
func (*visibilityActor) HeldItems() (item.Stack, item.Stack) { return item.Stack{}, item.Stack{} }
func (*visibilityActor) Effects() []effect.Effect            { return nil }
func (*visibilityActor) Sneaking() bool                      { return false }
func (*visibilityActor) Sprinting() bool                     { return false }
func (*visibilityActor) Swimming() bool                      { return false }
func (*visibilityActor) Crawling() bool                      { return false }
func (*visibilityActor) Gliding() bool                       { return false }
func (a *visibilityActor) Close() error                      { return a.tx.RemoveEntity(a).Close() }

type visibilityActorType struct{}

func (*visibilityActor) UsingItem() bool   { return false }
func (a *visibilityActor) Invisible() bool { return a.data.Data.(*visibilityActorData).invisible }

func (visibilityActorType) EncodeEntity() string        { return "minecraft:player" }
func (visibilityActorType) BBox(world.Entity) cube.BBox { return cube.Box(-.3, 0, -.3, .3, 1.8, .3) }
func (visibilityActorType) Open(tx *world.Tx, h *world.EntityHandle, data *world.EntityData) world.Entity {
	return &visibilityActor{tx: tx, handle: h, data: data}
}
func (visibilityActorType) DecodeNBT(map[string]any, *world.EntityData) {}
func (visibilityActorType) EncodeNBT(*world.EntityData) map[string]any  { return nil }
func (visibilityActorType) Apply(data *world.EntityData) {
	armour := inventory.NewArmour(nil)
	armour.SetHelmet(item.NewStack(item.Helmet{Tier: item.ArmourTierIron{}}, 1))
	data.Data = &visibilityActorData{mode: world.GameModeSurvival, armour: armour}
}

func visibilitySession(tx *world.Tx, t *testing.T) (*Session, *visibilityActor) {
	t.Helper()
	observer := tx.AddEntity((world.EntitySpawnOpts{Position: mgl64.Vec3{.5, 64, .5}}).New(visibilityActorType{}, visibilityActorType{})).(*visibilityActor)
	s := &Session{
		conf: Config{Log: slog.Default()}, ent: observer.H(), br: world.DefaultBlockRegistry,
		conn: &chunkVisibilityConn{}, currentEntityRuntimeID: selfEntityRuntimeID,
		entityRuntimeIDs: map[*world.EntityHandle]uint64{observer.H(): selfEntityRuntimeID},
		entities:         map[uint64]*world.EntityHandle{selfEntityRuntimeID: observer.H()},
		hiddenEntities:   map[uuid.UUID]struct{}{}, packets: make(chan outboundMessage, 4096),
		blobs: map[uint64][]byte{}, chunkTransactions: map[world.ChunkPos]map[uint64]struct{}{},
		closeBackground: make(chan struct{}), chunkRadius: 2, chunkEncoding: chunk.NetworkEncoding,
	}
	s.chunkLoader = world.NewLoader(2, tx.World(), s)
	s.chunkLoader.Move(tx, observer.Position())
	return s, observer
}

func drainVisibilityPackets(s *Session) []packet.Packet {
	var out []packet.Packet
	for len(s.packets) > 0 {
		out = append(out, (<-s.packets).packet)
	}
	return out
}

func countPlayerSpawns(packets []packet.Packet, id uuid.UUID) int {
	n := 0
	for _, pk := range packets {
		if add, ok := pk.(*packet.AddPlayer); ok && add.UUID == id {
			n++
		}
	}
	return n
}

func TestPlayerUnhideWaitsForDeliveredChunk(t *testing.T) {
	w := world.Config{Synchronous: true}.New()
	t.Cleanup(func() { _ = w.Close() })
	if err := w.Do(func(tx *world.Tx) {
		s, _ := visibilitySession(tx, t)
		defer s.chunkLoader.Close(tx)
		s.chunkLoader.Load(tx, 32)
		for _, x := range []float64{300.5, 301.5} {
			subject := tx.AddEntity((world.EntitySpawnOpts{Position: mgl64.Vec3{x, 64, .5}}).New(visibilityActorType{}, visibilityActorType{})).(*visibilityActor)
			s.StopShowingEntity(subject)
			drainVisibilityPackets(s)
			s.StartShowingEntity(subject)
			if countPlayerSpawns(drainVisibilityPackets(s), subject.UUID()) != 0 {
				t.Fatal("player unhide sent AddPlayer before the viewer received the destination chunk")
			}
		}
	}).Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestPlayerSpawnAndRemovalAreIdempotent(t *testing.T) {
	w := world.Config{Synchronous: true}.New()
	t.Cleanup(func() { _ = w.Close() })
	if err := w.Do(func(tx *world.Tx) {
		s, _ := visibilitySession(tx, t)
		defer s.chunkLoader.Close(tx)
		s.chunkLoader.Load(tx, 32)
		drainVisibilityPackets(s)
		subject := tx.AddEntity((world.EntitySpawnOpts{Position: mgl64.Vec3{1.5, 64, .5}}).New(visibilityActorType{}, visibilityActorType{})).(*visibilityActor)
		s.ViewEntity(subject)
		if n := countPlayerSpawns(drainVisibilityPackets(s), subject.UUID()); n != 1 {
			t.Fatalf("overlapping visibility callbacks emitted %d AddPlayer packets, want 1", n)
		}
		s.HideEntity(subject)
		s.HideEntity(subject)
		removed := 0
		for _, pk := range drainVisibilityPackets(s) {
			if _, ok := pk.(*packet.RemoveActor); ok {
				removed++
			}
		}
		if removed != 1 {
			t.Fatalf("duplicate hide emitted %d RemoveActor packets, want 1", removed)
		}
	}).Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func resolveVisibilityCache(t *testing.T, s *Session, tx *world.Tx, c Controllable, misses bool) {
	t.Helper()
	status := &packet.ClientCacheBlobStatus{}
	for hash := range s.blobs {
		if misses {
			status.MissHashes = append(status.MissHashes, hash)
		} else {
			status.HitHashes = append(status.HitHashes, hash)
		}
	}
	if err := (&ClientCacheBlobStatusHandler{}).Handle(status, s, tx, c); err != nil {
		t.Fatal(err)
	}
	s.flushPendingPlayers(tx)
}

func assertVisiblePair(t *testing.T, packets []packet.Packet, subjects []*visibilityActor, invisible bool) {
	t.Helper()
	for _, subject := range subjects {
		var spawn *packet.AddPlayer
		var armour *packet.MobArmourEquipment
		for _, pk := range packets {
			switch pk := pk.(type) {
			case *packet.AddPlayer:
				if pk.UUID == subject.UUID() {
					if spawn != nil {
						t.Fatal("duplicate player spawn")
					}
					spawn = pk
				}
			case *packet.MobArmourEquipment:
				if spawn != nil && pk.EntityRuntimeID == spawn.EntityRuntimeID {
					armour = pk
				}
			}
		}
		if spawn == nil || armour == nil {
			t.Fatalf("player %s missing body or equipment: spawn=%v armour=%v", subject.UUID(), spawn != nil, armour != nil)
		}
		if spawn.EntityMetadata[protocol.EntityDataKeyName] != subject.NameTag() || spawn.EntityMetadata.Flag(protocol.EntityDataKeyFlags, protocol.EntityDataFlagInvisible) != invisible {
			t.Fatal("spawn lost current nametag or invisibility state")
		}
		if armour.Helmet.Stack.NetworkID == 0 {
			t.Fatal("spawn lost equipped helmet")
		}
	}
}

func TestTwoPlayerVisibilityLifecycleMatrix(t *testing.T) {
	for _, cache := range []bool{false, true} {
		for _, misses := range []bool{false, true} {
			t.Run(fmt.Sprintf("cache=%t/misses=%t", cache, misses), func(t *testing.T) {
				w := world.Config{Synchronous: true}.New()
				t.Cleanup(func() { _ = w.Close() })
				if err := w.Do(func(tx *world.Tx) {
					s, observer := visibilitySession(tx, t)
					s.conn = &chunkVisibilityConn{cache: cache}
					defer s.chunkLoader.Close(tx)
					var subjects []*visibilityActor
					for i := range 2 {
						subject := tx.AddEntity((world.EntitySpawnOpts{Position: mgl64.Vec3{float64(i) + 1.5, 64, .5}}).New(visibilityActorType{}, visibilityActorType{})).(*visibilityActor)
						subject.data.Name = fmt.Sprintf("Green%d", i)
						subjects = append(subjects, subject)
						s.StopShowingEntity(subject)
						s.StartShowingEntity(subject)
					}
					for _, subject := range subjects {
						if countPlayerSpawns(drainVisibilityPackets(s), subject.UUID()) != 0 {
							t.Fatal("spawn before chunk delivery")
						}
					}
					s.chunkLoader.Load(tx, 32)
					if cache {
						for _, pk := range drainVisibilityPackets(s) {
							if _, ok := pk.(*packet.AddPlayer); ok {
								t.Fatal("spawn before chunk cache resolution")
							}
						}
					}
					resolveVisibilityCache(t, s, tx, observer, misses)
					assertVisiblePair(t, drainVisibilityPackets(s), subjects, false)

					// The viewer leaves and returns without reconnecting. Both cached
					// and uncached chunks must recreate both actors, every time.
					for range 3 {
						s.chunkLoader.Move(tx, mgl64.Vec3{300.5, 64, .5})
						s.chunkLoader.Load(tx, 32)
						resolveVisibilityCache(t, s, tx, observer, misses)
						drainVisibilityPackets(s)
						s.chunkLoader.Move(tx, observer.Position())
						s.chunkLoader.Load(tx, 32)
						resolveVisibilityCache(t, s, tx, observer, misses)
						assertVisiblePair(t, drainVisibilityPackets(s), subjects, false)
					}

					for _, mode := range []world.GameMode{world.GameModeNativeSpectator, world.GameModeSpectator} {
						for _, subject := range subjects {
							subject.data.Data.(*visibilityActorData).mode = mode
							s.HideEntity(subject)
							s.StopShowingEntity(subject)
						}
						drainVisibilityPackets(s)
						for _, subject := range subjects {
							s.ViewEntity(subject)
							s.ViewEntityState(subject)
							s.ViewEntityGameMode(subject)
							s.ViewEntityArmour(subject)
							s.ViewEntityMovement(subject, subject.Position(), cube.Rotation{}, true)
						}
						if len(drainVisibilityPackets(s)) != 0 {
							t.Fatal("spectator received actor updates after removal")
						}
						for _, subject := range subjects {
							subject.data.Data.(*visibilityActorData).mode = world.GameModeSurvival
							s.StartShowingEntity(subject)
							s.ViewEntity(subject) // A simultaneous chunk callback is harmless.
						}
						assertVisiblePair(t, drainVisibilityPackets(s), subjects, false)
					}

					// Effect metadata must remain authoritative through a full respawn.
					for _, subject := range subjects {
						s.StopShowingEntity(subject)
						subject.data.Data.(*visibilityActorData).invisible = true
						s.StartShowingEntity(subject)
					}
					assertVisiblePair(t, drainVisibilityPackets(s), subjects, true)
				}).Wait(t.Context()); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestPendingPlayerSpawnIsCancelledByHideRemovalAndWorldChange(t *testing.T) {
	w := world.Config{Synchronous: true}.New()
	other := world.Config{Synchronous: true}.New()
	t.Cleanup(func() { _ = other.Close(); _ = w.Close() })
	var s *Session
	var observerHandle, subjectHandle *world.EntityHandle
	if err := w.Do(func(tx *world.Tx) {
		var observer *visibilityActor
		s, observer = visibilitySession(tx, t)
		subject := tx.AddEntity((world.EntitySpawnOpts{Position: mgl64.Vec3{1.5, 64, .5}}).New(visibilityActorType{}, visibilityActorType{})).(*visibilityActor)
		s.ViewEntity(subject)
		s.StopShowingEntity(subject)
		s.chunkLoader.Load(tx, 32)
		s.flushPendingPlayers(tx)
		if countPlayerSpawns(drainVisibilityPackets(s), subject.UUID()) != 0 {
			t.Fatal("pending hidden player spawned")
		}
		s.StartShowingEntity(subject)
		drainVisibilityPackets(s)
		s.HideEntity(subject)
		s.chunkLoader.Move(tx, mgl64.Vec3{300.5, 64, .5})
		s.ViewEntity(subject)
		subjectHandle = tx.RemoveEntity(subject)
		s.flushPendingPlayers(tx)
		if len(s.pendingPlayers) != 0 {
			t.Fatal("departed actor retained pending spawn")
		}
		observerHandle = tx.RemoveEntity(observer)
	}).Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := other.Do(func(tx *world.Tx) {
		observer := tx.AddEntityAt(observerHandle, mgl64.Vec3{.5, 64, .5}).(*visibilityActor)
		subject := tx.AddEntityAt(subjectHandle, mgl64.Vec3{1.5, 64, .5}).(*visibilityActor)
		s.ViewEntity(subject)
		s.chunkLoader.ChangeWorld(tx, other)
		s.chunkLoader.Move(tx, observer.Position())
		drainVisibilityPackets(s)
		s.sendChunks(tx, observer)
		if countPlayerSpawns(drainVisibilityPackets(s), subject.UUID()) != 1 {
			t.Fatal("world change did not deliver exactly one current actor")
		}
		s.chunkLoader.Close(tx)
	}).Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestPlayerVisibilityIsIndependentPerViewerAndReconnect(t *testing.T) {
	w := world.Config{Synchronous: true}.New()
	t.Cleanup(func() { _ = w.Close() })
	if err := w.Do(func(tx *world.Tx) {
		fast, _ := visibilitySession(tx, t)
		slow, observer := visibilitySession(tx, t)
		slow.conn = &chunkVisibilityConn{cache: true}
		defer fast.chunkLoader.Close(tx)
		defer slow.chunkLoader.Close(tx)
		var subjects []*visibilityActor
		for i := range 2 {
			subjects = append(subjects, tx.AddEntity((world.EntitySpawnOpts{ID: uuid.New(), NameTag: fmt.Sprintf("Green%d", i), Position: mgl64.Vec3{float64(i) + 1.5, 64, .5}}).New(visibilityActorType{}, visibilityActorType{})).(*visibilityActor))
		}
		fast.chunkLoader.Load(tx, 32)
		slow.chunkLoader.Load(tx, 32)
		assertVisiblePair(t, drainVisibilityPackets(fast), subjects, false)
		for _, pk := range drainVisibilityPackets(slow) {
			if _, ok := pk.(*packet.AddPlayer); ok {
				t.Fatal("slow viewer received a player before cache resolution")
			}
		}
		resolveVisibilityCache(t, slow, tx, observer, true)
		assertVisiblePair(t, drainVisibilityPackets(slow), subjects, false)

		// A viewer dying must not remove opponents. The subject's mode, not the
		// observer's mode, decides actor visibility.
		observer.data.Data.(*visibilityActorData).mode = world.GameModeNativeSpectator
		for _, subject := range subjects {
			slow.ViewEntity(subject)
			slow.ViewEntityState(subject)
		}
		for _, pk := range drainVisibilityPackets(slow) {
			if _, ok := pk.(*packet.RemoveActor); ok {
				t.Fatal("observer death hid an active opponent")
			}
		}
		observer.data.Data.(*visibilityActorData).mode = world.GameModeSurvival

		oldID := fast.entityRuntimeID(subjects[0])
		old := tx.RemoveEntity(subjects[0])
		if err := old.Close(); err != nil {
			t.Fatal(err)
		}
		subjects[0] = tx.AddEntity((world.EntitySpawnOpts{ID: old.UUID(), NameTag: "RejoinedGreen", Position: mgl64.Vec3{1.5, 64, .5}}).New(visibilityActorType{}, visibilityActorType{})).(*visibilityActor)
		if id := fast.entityRuntimeID(subjects[0]); id == oldID || id == 0 {
			t.Fatal("reconnected handle reused stale runtime ID")
		}
		for _, s := range []*Session{fast, slow} {
			packets := drainVisibilityPackets(s)
			assertVisiblePair(t, packets, subjects[:1], false)
			if countPlayerSpawns(packets, subjects[1].UUID()) != 0 {
				t.Fatal("one reconnect recreated the other player")
			}
			if !s.entityShown(subjects[1]) {
				t.Fatal("one reconnect hid the other player")
			}
		}
	}).Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
}

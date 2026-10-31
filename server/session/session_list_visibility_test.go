package session

import (
	"testing"
	"time"

	"github.com/df-mc/dragonfly/server/player/skin"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/login"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

type playerListIdentityConn struct {
	chunkVisibilityConn
	identity login.IdentityData
}

func (c *playerListIdentityConn) IdentityData() login.IdentityData { return c.identity }

func TestPlayerListUsesSubjectPlayFabIdentity(t *testing.T) {
	subject := playerListTestSession(uuid.New())
	viewer := playerListTestSession(uuid.New())
	t.Cleanup(func() { _ = subject.ent.Close(); _ = viewer.ent.Close() })
	subject.conn = &playerListIdentityConn{identity: login.IdentityData{PlayFabID: "subject-playfab"}}
	viewer.conn = &playerListIdentityConn{identity: login.IdentityData{PlayFabID: "viewer-playfab"}}
	list := new(sessionList)
	list.sendSessionTo(subject, viewer)
	packets := drainVisibilityPackets(viewer)
	if len(packets) != 1 {
		t.Fatalf("player-list packet count = %d", len(packets))
	}
	pk, ok := packets[0].(*packet.PlayerList)
	if !ok || len(pk.Entries) != 1 {
		t.Fatalf("player list = %#v", packets[0])
	}
	if entry := pk.Entries[0]; entry.UUID != subject.ent.UUID() || entry.PlayFabID != "subject-playfab" {
		t.Fatalf("subject identity was not retained: %+v", entry)
	}
}

func playerListTestSession(id uuid.UUID) *Session {
	handle := world.EntitySpawnOpts{ID: id}.New(visibilityActorType{}, visibilityActorType{})
	return &Session{
		ent: handle, conn: &chunkVisibilityConn{}, joinSkin: skin.New(64, 64),
		currentEntityRuntimeID: selfEntityRuntimeID,
		entityRuntimeIDs:       map[*world.EntityHandle]uint64{handle: selfEntityRuntimeID},
		entities:               map[uint64]*world.EntityHandle{selfEntityRuntimeID: handle},
		shownEntities:          map[*world.EntityHandle]struct{}{}, pendingPlayers: map[*world.EntityHandle]struct{}{},
		packets: make(chan outboundMessage, 256), closeBackground: make(chan struct{}),
	}
}

func TestPlayerListLateRemovalPreservesReplacement(t *testing.T) {
	list := new(sessionList)
	observer := playerListTestSession(uuid.New())
	old := playerListTestSession(uuid.New())
	replacement := playerListTestSession(old.ent.UUID())
	for _, s := range []*Session{observer, old, replacement} {
		t.Cleanup(func() { _ = s.ent.Close() })
	}
	list.Add(observer)
	list.Add(old)
	oldID := observer.entityRuntimeIDs[old.ent]
	observer.shownEntities[old.ent] = struct{}{}
	drainVisibilityPackets(observer)

	// A backend return may register its new session after the old session left
	// Server's online map but before that session finishes world/list cleanup.
	list.Add(replacement)
	if current, ok := list.Lookup(old.ent.UUID()); !ok || current != replacement {
		t.Error("UUID lookup still selects the departing session")
	}
	listed := map[uuid.UUID]int64{old.ent.UUID(): int64(oldID)}
	actors := map[int64]bool{int64(oldID): true}
	apply := func(packets []packet.Packet) {
		for _, raw := range packets {
			switch pk := raw.(type) {
			case *packet.PlayerList:
				for _, entry := range pk.Entries {
					if entry.ActionType == protocol.PlayerListActionAdd {
						listed[entry.UUID] = entry.EntityUniqueID
					} else {
						delete(listed, entry.UUID)
					}
				}
			case *packet.RemoveActor:
				delete(actors, pk.EntityUniqueID)
			}
		}
	}
	apply(drainVisibilityPackets(observer))
	if actors[int64(oldID)] {
		t.Error("replacement left the previous actor on the client")
	}
	newID := observer.entityRuntimeIDs[replacement.ent]
	for _, s := range []*Session{old, replacement} {
		if s.entityRuntimeIDs[s.ent] != selfEntityRuntimeID || s.listedPlayers[s.ent.UUID()] != s.ent {
			t.Error("overlapping login replaced the controlling connection's own identity")
		}
	}
	list.Remove(old, nil)
	apply(drainVisibilityPackets(observer))
	if got := listed[replacement.ent.UUID()]; got != int64(newID) || newID == 0 {
		t.Errorf("late cleanup erased replacement identity: got %d, want %d", got, newID)
	}
	list.Remove(replacement, nil)
	apply(drainVisibilityPackets(observer))
	if _, present := listed[replacement.ent.UUID()]; present {
		t.Error("final departure retained the replacement identity")
	}
}

func TestRegisteredPlayerRespawnReassertsIdentity(t *testing.T) {
	w := world.Config{Synchronous: true}.New()
	t.Cleanup(func() { _ = w.Close() })
	if err := w.Do(func(tx *world.Tx) {
		observer, _ := visibilitySession(tx, t)
		observer.chunkLoader.Load(tx, 32)
		defer observer.chunkLoader.Close(tx)
		sessions.Add(observer)
		defer sessions.Remove(observer, nil)
		old := playerListTestSession(uuid.New())
		replacement := playerListTestSession(old.ent.UUID())
		sessions.Add(old)
		defer sessions.Remove(old, nil)
		defer sessions.Remove(replacement, nil)
		drainVisibilityPackets(observer)
		subject := tx.AddEntity(old.ent).(*visibilityActor)
		assertRegisteredSpawn(t, drainVisibilityPackets(observer), subject.UUID())
		for range 3 {
			observer.HideEntity(subject)
			drainVisibilityPackets(observer)
			observer.ViewEntity(subject)
			assertRegisteredSpawn(t, drainVisibilityPackets(observer), subject.UUID())
		}
		observer.StopShowingEntity(subject)
		drainVisibilityPackets(observer)
		observer.ViewEntity(subject)
		if got := drainVisibilityPackets(observer); len(got) != 0 {
			t.Fatal("hidden participant was exposed by identity repair")
		}
		observer.StartShowingEntity(subject)
		assertRegisteredSpawn(t, drainVisibilityPackets(observer), subject.UUID())

		sessions.Add(replacement)
		drainVisibilityPackets(observer)
		observer.ViewEntity(subject)
		observer.ViewSkin(subject)
		if got := drainVisibilityPackets(observer); len(got) != 0 {
			t.Fatal("retired session rewrote its replacement's actor or skin")
		}
		current := tx.AddEntity(replacement.ent).(*visibilityActor)
		assertRegisteredSpawn(t, drainVisibilityPackets(observer), current.UUID())
		sessions.Remove(old, nil)
		_ = tx.RemoveEntity(subject).Close()
		if got := drainVisibilityPackets(observer); len(got) != 0 {
			t.Fatal("late world/list teardown removed the replacement")
		}
		if !observer.entityShown(current) {
			t.Fatal("replacement stopped being visible after old teardown")
		}
	}).Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func assertRegisteredSpawn(t *testing.T, packets []packet.Packet, id uuid.UUID) {
	t.Helper()
	spawned := false
	for i, raw := range packets {
		if list, ok := raw.(*packet.PlayerList); ok {
			for _, entry := range list.Entries {
				if entry.UUID == id && entry.ActionType == protocol.PlayerListActionRemove {
					t.Fatal("real player spawn removed its persistent identity")
				}
			}
		}
		pk, ok := raw.(*packet.AddPlayer)
		if !ok || pk.UUID != id {
			continue
		}
		if spawned || i == 0 {
			t.Fatal("duplicate spawn or missing identity before AddPlayer")
		}
		list, ok := packets[i-1].(*packet.PlayerList)
		if !ok || len(list.Entries) != 1 {
			t.Fatal("AddPlayer was not immediately preceded by its current identity")
		}
		entry := list.Entries[0]
		if entry.UUID != id || entry.EntityUniqueID != pk.AbilityData.EntityUniqueID || entry.EntityUniqueID != int64(pk.EntityRuntimeID) || len(entry.Skin.SkinData) == 0 {
			t.Fatal("spawn identity, runtime ID or skin disagrees with PlayerList")
		}
		spawned = true
	}
	if !spawned {
		t.Fatal("registered player was never spawned")
	}
}

type preparingVisibilityActor struct {
	*visibilityActor
	prepare func()
}

func (a preparingVisibilityActor) Skin() skin.Skin {
	a.prepare()
	return a.visibilityActor.Skin()
}

func TestPlayerReplacementDuringSpawnPreparation(t *testing.T) {
	w := world.Config{Synchronous: true}.New()
	t.Cleanup(func() { _ = w.Close() })
	if err := w.Do(func(tx *world.Tx) {
		observer, _ := visibilitySession(tx, t)
		observer.chunkLoader.Load(tx, 32)
		defer observer.chunkLoader.Close(tx)
		sessions.Add(observer)
		defer sessions.Remove(observer, nil)
		old := playerListTestSession(uuid.New())
		replacement := playerListTestSession(old.ent.UUID())
		sessions.Add(old)
		defer sessions.Remove(old, nil)
		defer sessions.Remove(replacement, nil)
		defer replacement.ent.Close()
		subject := tx.AddEntity(old.ent).(*visibilityActor)
		observer.HideEntity(subject)
		drainVisibilityPackets(observer)
		prepared, replaced := make(chan struct{}), make(chan struct{})
		go func() {
			<-prepared
			sessions.Add(replacement)
			sessions.Remove(old, nil)
			close(replaced)
		}()
		observer.ViewEntity(preparingVisibilityActor{visibilityActor: subject, prepare: func() {
			close(prepared)
			select {
			case <-replaced:
			case <-time.After(5 * time.Second):
				t.Fatal("replacement deadlocked while spawn metadata was being prepared")
			}
		}})
		for _, raw := range drainVisibilityPackets(observer) {
			if _, ok := raw.(*packet.AddPlayer); ok {
				t.Fatal("old spawn was queued after its replacement's identity")
			}
		}
		if observer.listedPlayers[subject.UUID()] != replacement.ent {
			t.Fatal("in-flight spawn overwrote the replacement's identity")
		}
	}).Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
}

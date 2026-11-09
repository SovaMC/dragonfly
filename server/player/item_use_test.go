package player

import (
	"testing"
	"time"

	"github.com/df-mc/dragonfly/server/item"
	"github.com/df-mc/dragonfly/server/session"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl64"
	"github.com/google/uuid"
)

type itemUseCountingHandler struct {
	NopHandler
	uses, consumes int
	cancelConsume  bool
	cancelRelease  bool
}

type itemUseStateViewer struct {
	world.NopViewer
	states int
}

func (v *itemUseStateViewer) ViewEntityState(world.Entity) {
	v.states++
}

func (h *itemUseCountingHandler) HandleItemUse(*Context) { h.uses++ }

func (h *itemUseCountingHandler) HandleItemConsume(ctx *Context, _ item.Stack) {
	h.consumes++
	if h.cancelConsume {
		ctx.Cancel()
	}
}

func (h *itemUseCountingHandler) HandleItemRelease(ctx *Context, _ item.Stack, _ time.Duration) {
	if h.cancelRelease {
		ctx.Cancel()
	}
}

func TestRepeatedReleasableUsePreservesStart(t *testing.T) {
	w := world.Config{Synchronous: true}.New()
	t.Cleanup(func() { _ = w.Close() })

	if err := w.Do(func(tx *world.Tx) {
		pos := mgl64.Vec3{0.5, 64, 0.5}
		pl := tx.AddEntity(world.EntitySpawnOpts{Position: pos}.New(Type, Config{
			UUID: uuid.New(), Name: "Archer", Position: pos,
		})).(*Player)
		_ = pl.Inventory().SetItem(0, item.NewStack(item.Bow{}, 1))
		_ = pl.Inventory().SetItem(1, item.NewStack(item.Arrow{}, 1))

		pl.UseItem()
		started := pl.usingSince
		if !pl.usingItem {
			t.Fatal("bow use did not start")
		}
		pl.UseItem()
		if !pl.usingItem || !pl.usingSince.Equal(started) {
			t.Fatal("repeated bow use restarted or stopped the release state")
		}
		pl.ReleaseItem()
		if pl.usingItem {
			t.Fatal("bow release did not complete the use cycle")
		}
		pl.usingSince = time.Time{}
		pl.UseItem()
		if !pl.usingItem || pl.usingSince.IsZero() {
			t.Fatal("continued input did not start a new bow use cycle")
		}
	}).Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestEarlyChargeRepeatKeepsCharging(t *testing.T) {
	w := world.Config{Synchronous: true}.New()
	t.Cleanup(func() { _ = w.Close() })

	if err := w.Do(func(tx *world.Tx) {
		pos := mgl64.Vec3{0.5, 64, 0.5}
		pl := tx.AddEntity(world.EntitySpawnOpts{Position: pos}.New(Type, Config{
			UUID: uuid.New(), Name: "Crossbowman", Position: pos,
		})).(*Player)
		handler := &itemUseCountingHandler{}
		pl.Handle(handler)
		_ = pl.Inventory().SetItem(0, item.NewStack(item.Crossbow{}, 1))
		_ = pl.Inventory().SetItem(1, item.NewStack(item.Arrow{}, 1))

		pl.UseItem()
		if !pl.usingItem {
			t.Fatal("crossbow charge did not start")
		}
		pl.UseItem()
		if !pl.usingItem {
			t.Fatal("early crossbow repeat stopped charging")
		}
		if handler.uses != 1 {
			t.Fatalf("item-use handler calls = %d, want 1", handler.uses)
		}
	}).Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestConsumableRepeatsKeepUpstreamContinuousUse(t *testing.T) {
	w := world.Config{Synchronous: true}.New()
	t.Cleanup(func() { _ = w.Close() })

	if err := w.Do(func(tx *world.Tx) {
		pos := mgl64.Vec3{0.5, 64, 0.5}
		pl := tx.AddEntity(world.EntitySpawnOpts{Position: pos}.New(Type, Config{
			UUID: uuid.New(), Name: "Eater", Position: pos, Food: 10,
		})).(*Player)
		handler := &itemUseCountingHandler{}
		pl.Handle(handler)
		_ = pl.Inventory().SetItem(0, item.NewStack(item.Apple{}, 2))

		pl.UseItem()
		if !pl.usingItem || handler.uses != 1 {
			t.Fatalf("initial consumption state = %v, uses = %d", pl.usingItem, handler.uses)
		}
		pl.usingSince = time.Now()
		pl.UseItem()
		held, _ := pl.HeldItems()
		if held.Count() != 2 || handler.uses != 2 || handler.consumes != 0 {
			t.Fatalf("early repeat changed stack/events: count=%d uses=%d consumes=%d", held.Count(), handler.uses, handler.consumes)
		}

		pl.usingSince = time.Now().Add(-2 * time.Second)
		pl.UseItem()
		held, _ = pl.HeldItems()
		if !pl.usingItem || held.Count() != 1 || handler.uses != 3 || handler.consumes != 1 {
			t.Fatalf("completed consume state=%v count=%d uses=%d consumes=%d", pl.usingItem, held.Count(), handler.uses, handler.consumes)
		}

		pl.UseItem()
		if !pl.usingItem || handler.uses != 4 || handler.consumes != 1 {
			t.Fatalf("held input bypassed the next duration: state=%v uses=%d consumes=%d", pl.usingItem, handler.uses, handler.consumes)
		}
		pl.usingSince = time.Now().Add(-2 * time.Second)
		pl.UseItem()
		held, _ = pl.HeldItems()
		if pl.usingItem || !held.Empty() || handler.consumes != 2 {
			t.Fatalf("final consume state=%v held=%+v consumes=%d", pl.usingItem, held, handler.consumes)
		}
		pl.ReleaseItem()
		if pl.usingItem {
			t.Fatal("release did not stop consumption")
		}
	}).Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestCancelledConsumptionRetainsUpstreamUseAndResetsDuration(t *testing.T) {
	w := world.Config{Synchronous: true}.New()
	t.Cleanup(func() { _ = w.Close() })

	if err := w.Do(func(tx *world.Tx) {
		pos := mgl64.Vec3{0.5, 64, 0.5}
		pl := tx.AddEntity(world.EntitySpawnOpts{Position: pos}.New(Type, Config{
			UUID: uuid.New(), Name: "Eater", Position: pos, Food: 10,
		})).(*Player)
		handler := &itemUseCountingHandler{cancelConsume: true}
		pl.Handle(handler)
		_ = pl.Inventory().SetItem(0, item.NewStack(item.Apple{}, 2))

		pl.UseItem()
		pl.usingSince = time.Now().Add(-2 * time.Second)
		pl.UseItem()
		held, _ := pl.HeldItems()
		if !pl.usingItem || held.Count() != 2 || handler.consumes != 1 {
			t.Fatalf("cancelled consume state=%v count=%d consumes=%d", pl.usingItem, held.Count(), handler.consumes)
		}

		// Upstream keeps consuming after cancellation but restarts its timer.
		pl.UseItem()
		pl.UseItem()
		if !pl.usingItem || handler.consumes != 1 {
			t.Fatalf("fresh consume attempt state=%v consumes=%d", pl.usingItem, handler.consumes)
		}
		pl.ReleaseItem()
		if pl.usingItem {
			t.Fatal("release did not stop the cancelled consumption")
		}
	}).Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestCancelledBowReleaseEndsUseCycle(t *testing.T) {
	w := world.Config{Synchronous: true}.New()
	t.Cleanup(func() { _ = w.Close() })

	if err := w.Do(func(tx *world.Tx) {
		pos := mgl64.Vec3{0.5, 64, 0.5}
		pl := tx.AddEntity(world.EntitySpawnOpts{Position: pos}.New(Type, Config{
			UUID: uuid.New(), Name: "Archer", Position: pos,
		})).(*Player)
		pl.Handle(&itemUseCountingHandler{cancelRelease: true})
		_ = pl.Inventory().SetItem(0, item.NewStack(item.Bow{}, 1))
		_ = pl.Inventory().SetItem(1, item.NewStack(item.Arrow{}, 1))

		pl.UseItem()
		pl.ReleaseItem()
		if pl.usingItem {
			t.Fatal("cancelled bow release left the use cycle active")
		}
		pl.UseItem()
		if !pl.usingItem {
			t.Fatal("new bow press did not start after a cancelled release")
		}
	}).Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestFullFoodStartsOnlyAlwaysConsumableItems(t *testing.T) {
	w := world.Config{Synchronous: true}.New()
	t.Cleanup(func() { _ = w.Close() })

	if err := w.Do(func(tx *world.Tx) {
		pos := mgl64.Vec3{0.5, 64, 0.5}
		pl := tx.AddEntity(world.EntitySpawnOpts{Position: pos}.New(Type, Config{
			UUID: uuid.New(), Name: "Full", Position: pos, Food: 20,
		})).(*Player)

		_ = pl.Inventory().SetItem(0, item.NewStack(item.Apple{}, 1))
		pl.UseItem()
		if pl.usingItem {
			t.Fatal("ordinary food started at full hunger")
		}

		_ = pl.Inventory().SetItem(0, item.NewStack(item.GoldenApple{}, 1))
		pl.UseItem()
		if !pl.usingItem {
			t.Fatal("always-consumable golden apple did not start at full hunger")
		}
	}).Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestClientPredictedItemUseStateExcludesOnlySelf(t *testing.T) {
	other := &itemUseStateViewer{}
	viewers := []world.Viewer{session.Nop, other}

	viewItemUseState(viewers, session.Nop, nil, true)
	if other.states != 1 {
		t.Fatalf("predicted update states other=%d", other.states)
	}

	first, second := &itemUseStateViewer{}, &itemUseStateViewer{}
	viewItemUseState([]world.Viewer{first, second}, session.Nop, nil, false)
	if first.states != 1 || second.states != 1 {
		t.Fatalf("server update states first=%d second=%d", first.states, second.states)
	}
}

// TestFauxSpectatorItemUseReachesHandler keeps hotbar controls of the clickable
// faux spectator working in air, while native spectator still uses nothing.
func TestFauxSpectatorItemUseReachesHandler(t *testing.T) {
	w := world.Config{Synchronous: true}.New()
	t.Cleanup(func() { _ = w.Close() })

	if err := w.Do(func(tx *world.Tx) {
		pos := mgl64.Vec3{0.5, 64, 0.5}
		pl := tx.AddEntity(world.EntitySpawnOpts{Position: pos}.New(Type, Config{
			UUID: uuid.New(), Name: "Spectator", Position: pos,
		})).(*Player)
		handler := &itemUseCountingHandler{}
		pl.Handle(handler)
		_ = pl.Inventory().SetItem(0, item.NewStack(item.Snowball{}, 16))

		pl.SetGameMode(world.GameModeSpectator)
		pl.UseItem()
		if handler.uses != 1 {
			t.Fatalf("faux spectator item-use handler calls = %d, want 1", handler.uses)
		}
		if held, _ := pl.HeldItems(); held.Count() != 16 {
			t.Fatalf("faux spectator used the item itself: %d snowballs left", held.Count())
		}
		pl.SetGameMode(world.GameModeNativeSpectator)
		pl.UseItem()
		if handler.uses != 1 {
			t.Fatalf("native spectator reached the item-use handler")
		}
	}).Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
}

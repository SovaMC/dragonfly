package session

import (
	"testing"
	"time"

	"github.com/df-mc/dragonfly/server/item"
	"github.com/df-mc/dragonfly/server/item/inventory"
	"github.com/go-gl/mathgl/mgl32"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func TestOffhandTransactionsDoNotUseMainHand(t *testing.T) {
	for _, hand := range []byte{protocol.HandSlotOffHand, 255} {
		for _, data := range []protocol.InventoryTransactionData{
			&protocol.UseItemOnEntityTransactionData{Hand: hand, HotBarSlot: 99},
			&protocol.ReleaseItemTransactionData{Hand: hand, HotBarSlot: 99},
		} {
			s := &Session{
				inv: inventory.New(36, nil), ui: inventory.New(54, nil), offHand: inventory.New(1, nil),
				armour: inventory.NewArmour(nil), packets: make(chan outboundMessage, 4), closeBackground: make(chan struct{}),
			}
			// A nil controllable and invalid main-hand slot must never be reached.
			err := (&InventoryTransactionHandler{}).Handle(&packet.InventoryTransaction{TransactionData: data}, s, nil, nil)
			if hand == protocol.HandSlotOffHand {
				if err != nil || len(s.packets) != 4 {
					t.Fatalf("%T offhand: err=%v resync packets=%d", data, err, len(s.packets))
				}
			} else if err == nil || len(s.packets) != 0 {
				t.Fatalf("%T invalid hand: err=%v packets=%d", data, err, len(s.packets))
			}
		}
	}
}

func TestRepeatedRightClickSignature(t *testing.T) {
	h := &InventoryTransactionHandler{}
	data := &protocol.UseItemTransactionData{
		BlockFace:       1,
		BlockPosition:   protocol.BlockPos{1, 2, 3},
		Position:        mgl32.Vec3{4, 5, 6},
		ClickedPosition: mgl32.Vec3{0.5, 1, 0.5},
	}
	now := time.Unix(100, 0)
	if h.repeatedRightClick(data, now) {
		t.Fatal("first click was classified as a repeat")
	}
	h.lastRightClick.blockHandled = true
	if !h.repeatedRightClick(data, now.Add(50*time.Millisecond)) {
		t.Fatal("identical handled click inside 100 ms was not filtered")
	}
	data.BlockPosition[0]++
	if h.repeatedRightClick(data, now.Add(75*time.Millisecond)) {
		t.Fatal("click on a different block was filtered")
	}
	if h.repeatedRightClick(data, now.Add(200*time.Millisecond)) {
		t.Fatal("click outside the 100 ms window was filtered")
	}
}

func TestUnhandledBlockRightClickRepeatsForAirFallback(t *testing.T) {
	h := &InventoryTransactionHandler{}
	data := &protocol.UseItemTransactionData{
		BlockFace:       1,
		BlockPosition:   protocol.BlockPos{1, 2, 3},
		Position:        mgl32.Vec3{4, 5, 6},
		ClickedPosition: mgl32.Vec3{0.5, 1, 0.5},
	}
	now := time.Unix(100, 0)
	if h.repeatedRightClick(data, now) || h.repeatedRightClick(data, now.Add(50*time.Millisecond)) {
		t.Fatal("unhandled block click was filtered before air-use fallback")
	}
}

func TestAirUseFallbackItems(t *testing.T) {
	tests := []struct {
		name  string
		stack item.Stack
		want  bool
	}{
		{name: "bow", stack: item.NewStack(item.Bow{}, 1), want: true},
		{name: "crossbow", stack: item.NewStack(item.Crossbow{}, 1), want: true},
		{name: "food", stack: item.NewStack(item.GoldenApple{}, 1), want: true},
		{name: "plain item", stack: item.NewStack(item.Arrow{}, 1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := usesItemInAir(test.stack); got != test.want {
				t.Fatalf("usesItemInAir() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestLongUseRecoveryItems(t *testing.T) {
	tests := []struct {
		name  string
		stack item.Stack
		want  bool
	}{
		{name: "bow", stack: item.NewStack(item.Bow{}, 1), want: true},
		{name: "crossbow", stack: item.NewStack(item.Crossbow{}, 1), want: true},
		{name: "food", stack: item.NewStack(item.GoldenApple{}, 1), want: true},
		{name: "throwable", stack: item.NewStack(item.EnderPearl{}, 1)},
		{name: "plain item", stack: item.NewStack(item.Arrow{}, 1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := usesLongItem(test.stack); got != test.want {
				t.Fatalf("usesLongItem() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestSimulationTickAirUsePolicy(t *testing.T) {
	tests := []struct {
		name  string
		stack item.Stack
		using bool
		want  bool
	}{
		{name: "plain control", stack: item.NewStack(item.Arrow{}, 1), want: true},
		{name: "bow drawing", stack: item.NewStack(item.Bow{}, 1), using: true, want: true},
		{name: "bow residual after release", stack: item.NewStack(item.Bow{}, 1), want: true},
		{name: "crossbow charging", stack: item.NewStack(item.Crossbow{}, 1), using: true, want: false},
		{name: "crossbow charged", stack: item.NewStack(item.Crossbow{Item: item.NewStack(item.Arrow{}, 1)}, 1), want: true},
		{name: "crossbow fired while held", stack: item.NewStack(item.Crossbow{}, 1), want: false},
		{name: "food consumption", stack: item.NewStack(item.Apple{}, 1), using: true, want: false},
		{name: "next food after consumption", stack: item.NewStack(item.Apple{}, 1), want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := skipAirSimulationTick(test.stack, test.using); got != test.want {
				t.Fatalf("skipAirSimulationTick() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestInteractionPredictionCompatibility(t *testing.T) {
	tests := []struct {
		name             string
		expected, actual item.Stack
		want             bool
	}{
		{name: "same stack", expected: item.NewStack(item.Apple{}, 2), actual: item.NewStack(item.Apple{}, 2), want: true},
		{name: "predicted count", expected: item.NewStack(item.Apple{}, 2), actual: item.NewStack(item.Apple{}, 1), want: true},
		{name: "predicted durability", expected: item.NewStack(item.Bow{}, 1), actual: item.NewStack(item.Bow{}, 1).Damage(1), want: true},
		{name: "different item", expected: item.NewStack(item.Apple{}, 1), actual: item.NewStack(item.Bow{}, 1), want: false},
		{name: "different custom data", expected: item.NewStack(item.Apple{}, 1).WithValue("variant", 1), actual: item.NewStack(item.Apple{}, 1).WithValue("variant", 2), want: false},
		{name: "empty mismatch", expected: item.Stack{}, actual: item.NewStack(item.Apple{}, 1), want: false},
		{name: "both empty", expected: item.Stack{}, actual: item.Stack{}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := interactionPredictionCompatible(test.expected, test.actual); got != test.want {
				t.Fatalf("interactionPredictionCompatible() = %v, want %v", got, test.want)
			}
		})
	}
}

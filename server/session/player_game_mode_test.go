package session

import (
	"testing"

	"github.com/df-mc/dragonfly/server/world"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func TestPocketMineSpectatorPresentation(t *testing.T) {
	if got := gameTypeFromMode(world.GameModeSpectator); got != packet.GameTypeCreative {
		t.Fatalf("spectator game type = %d, want creative", got)
	}
	abilities := gameModeAbilities(world.GameModeSpectator)
	if abilities&protocol.AbilityInstantBuild == 0 || abilities&protocol.AbilityInvulnerable == 0 {
		t.Fatalf("spectator abilities = %#x, missing PMMP creative/invulnerable bits", abilities)
	}
	if abilities&protocol.AbilityMayFly != 0 {
		t.Fatalf("spectator abilities = %#x, PMMP locks flying instead of allowing a toggle", abilities)
	}
	forbidden := uint32(protocol.AbilityDoorsAndSwitches | protocol.AbilityOpenContainers | protocol.AbilityAttackPlayers | protocol.AbilityAttackMobs)
	if abilities&forbidden != 0 {
		t.Fatalf("spectator abilities = %#x, interaction bits must remain disabled", abilities)
	}
}

func TestNativeSpectatorPresentationUnchanged(t *testing.T) {
	if got := gameTypeFromMode(world.GameModeNativeSpectator); got != packet.GameTypeSpectator {
		t.Fatalf("native spectator game type = %d, want native spectator", got)
	}
	abilities := gameModeAbilities(world.GameModeNativeSpectator)
	if abilities&protocol.AbilityInstantBuild != 0 {
		t.Fatalf("native spectator abilities = %#x, instant build unexpectedly enabled", abilities)
	}
}

func TestSpectatorActorsHiddenFromOtherSessions(t *testing.T) {
	for _, mode := range []world.GameMode{world.GameModeSpectator, world.GameModeNativeSpectator} {
		if gameModeVisibleToSession(false, mode) {
			t.Fatalf("%T is visible to another session", mode)
		}
		if !gameModeVisibleToSession(true, mode) {
			t.Fatalf("%T is hidden from its controlling session", mode)
		}
	}
	for _, mode := range []world.GameMode{world.GameModeSurvival, world.GameModeCreative, world.GameModeAdventure} {
		if !gameModeVisibleToSession(false, mode) {
			t.Fatalf("%T is hidden from another session", mode)
		}
	}
}

// TestCollisionlessModesAdvertiseFlightImmediately pins the fix for a client that
// stayed grounded after entering spectator.
//
// SendAbilities used to derive flight purely from the current state and repair it
// with a deferred, re-entrant StartFlying. Entering a collisionless mode from the
// ground therefore published noclip with flight clear, and the client kept
// colliding with blocks and sending attacks until the player toggled flight
// themselves. Both spectator modes must report noclip and flight in the first
// packet regardless of the flying state they are entered from.
func TestCollisionlessModesAdvertiseFlightImmediately(t *testing.T) {
	for _, mode := range []world.GameMode{world.GameModeSpectator, world.GameModeNativeSpectator} {
		for _, flying := range []bool{false, true} {
			abilities := abilityValues(mode, flying)
			if abilities&protocol.AbilityNoClip == 0 {
				t.Fatalf("%T entered with flying=%v: abilities = %#x, missing noclip", mode, flying, abilities)
			}
			if abilities&protocol.AbilityFlying == 0 {
				t.Fatalf("%T entered with flying=%v: abilities = %#x, missing flight", mode, flying, abilities)
			}
		}
	}
}

func TestFauxSpectatorStagesFlightBeforeNoClip(t *testing.T) {
	abilities := fauxSpectatorTransitionAbilities(world.GameModeSpectator)
	if abilities&protocol.AbilityFlying == 0 {
		t.Fatalf("transition abilities = %#x, missing forced flight", abilities)
	}
	if abilities&protocol.AbilityNoClip != 0 {
		t.Fatalf("transition abilities = %#x, noclip must be introduced by the next stage", abilities)
	}
	if abilities&protocol.AbilityMayFly != 0 {
		t.Fatalf("transition abilities = %#x, faux spectator must keep the client toggle locked", abilities)
	}
}

func TestFauxSpectatorResyncWaitsForLaterClientInput(t *testing.T) {
	remaining := fauxSpectatorResyncInputDelay
	for input := uint32(1); input <= fauxSpectatorResyncInputDelay; input++ {
		var ready bool
		remaining, ready = nextFauxSpectatorResyncInput(remaining)
		if input < fauxSpectatorResyncInputDelay && ready {
			t.Fatalf("resync became ready after input %d, want %d", input, fauxSpectatorResyncInputDelay)
		}
		if input == fauxSpectatorResyncInputDelay && !ready {
			t.Fatalf("resync not ready after input %d", input)
		}
	}
	if remaining != 0 {
		t.Fatalf("remaining inputs = %d, want 0", remaining)
	}
}

// TestCollidingModesKeepFlightDrivenByState guards the other direction: a mode that
// collides must not gain noclip, and its flight must still follow the player's own
// toggle rather than being forced on.
func TestCollidingModesKeepFlightDrivenByState(t *testing.T) {
	for _, mode := range []world.GameMode{world.GameModeSurvival, world.GameModeCreative, world.GameModeAdventure} {
		if abilities := abilityValues(mode, false); abilities&protocol.AbilityNoClip != 0 {
			t.Fatalf("%T: abilities = %#x, noclip must stay off for a colliding mode", mode, abilities)
		}
		grounded, airborne := abilityValues(mode, false), abilityValues(mode, true)
		if grounded&protocol.AbilityFlying != 0 {
			t.Fatalf("%T: grounded abilities = %#x, flight must not be forced on", mode, grounded)
		}
		if mode.AllowsFlying() && airborne&protocol.AbilityFlying == 0 {
			t.Fatalf("%T: airborne abilities = %#x, flight must follow the player's toggle", mode, airborne)
		}
	}
}

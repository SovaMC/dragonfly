package session

import (
	"github.com/df-mc/dragonfly/server/world"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// PacketViolationWarningHandler handles the PacketViolationWarning packet. A
// client reports one for every duplicate ContainerOpen it receives, which
// confirms that a re-sent virtual window is open.
type PacketViolationWarningHandler struct{}

// Handle ...
func (PacketViolationWarningHandler) Handle(_ packet.Packet, s *Session, _ *world.Tx, _ Controllable) error {
	s.acknowledgeVirtualWindow()
	return nil
}

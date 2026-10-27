package session

import (
	"github.com/df-mc/dragonfly/server/world"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

// NetworkStackLatencyHandler handles the NetworkStackLatency packet. The
// client answers it once every packet sent before the request was processed,
// which is used to open virtual windows after their fake container block.
type NetworkStackLatencyHandler struct{}

// Handle ...
func (NetworkStackLatencyHandler) Handle(_ packet.Packet, s *Session, _ *world.Tx, _ Controllable) error {
	if pending := s.pendingVirtual.Load(); pending != nil {
		s.openVirtualWindow(pending)
	}
	return nil
}

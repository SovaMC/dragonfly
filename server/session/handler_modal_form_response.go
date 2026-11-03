package session

import (
	"fmt"
	"github.com/df-mc/dragonfly/server/player/form"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
	"sync"
	"sync/atomic"
	"time"
)

// ModalFormResponseHandler handles the ModalFormResponse packet.
type ModalFormResponseHandler struct {
	mu        sync.Mutex
	forms     map[uint32]form.Form
	currentID atomic.Uint32
	// sent holds when each form still awaiting a response was sent, and
	// answered when the last response arrived. Item use reads both so a held
	// click cannot queue more forms or reopen one as it closes.
	sent     map[uint32]time.Time
	answered time.Time
}

// Handle ...
func (h *ModalFormResponseHandler) Handle(p packet.Packet, _ *Session, tx *world.Tx, c Controllable) error {
	pk := p.(*packet.ModalFormResponse)

	h.mu.Lock()
	f, ok := h.forms[pk.FormID]
	delete(h.forms, pk.FormID)
	delete(h.sent, pk.FormID)
	h.answered = time.Now()
	h.mu.Unlock()

	resp, exists := pk.ResponseData.Value()
	if !ok {
		// A client may reply to a form that was superseded or evicted after
		// several forms were opened quickly. The response cannot be applied
		// safely, but stale client UI state must not close the whole session.
		return nil
	}
	if !exists || len(resp) == 0 {
		// The form was cancelled: The cross in the top right corner was clicked.
		resp = nil
	}
	if err := f.SubmitJSON(resp, c, tx); err != nil {
		return fmt.Errorf("error submitting form data: %w", err)
	}
	return nil
}

// formState reports whether a form sent less than awaitLimit before now is
// still unanswered, and when the last response arrived. The limit keeps a
// response the client never sends from blocking item use indefinitely.
func (h *ModalFormResponseHandler) formState(now time.Time, awaitLimit time.Duration) (awaiting bool, answered time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, sent := range h.sent {
		if now.Sub(sent) < awaitLimit {
			return true, h.answered
		}
	}
	return false, h.answered
}

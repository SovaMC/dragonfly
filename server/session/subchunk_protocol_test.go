package session

import (
	"testing"

	"github.com/sandertv/gophertunnel/minecraft"
)

type idProtocol struct {
	minecraft.Protocol
	id int32
}

func (p idProtocol) ID() int32 { return p.id }

func TestSupportsSubChunkRequests(t *testing.T) {
	for id, want := range map[int32]bool{419: false, 470: false, 471: true, 486: true, 975: true} {
		if got := supportsSubChunkRequests(idProtocol{id: id}); got != want {
			t.Errorf("protocol %d: got %v, want %v", id, got, want)
		}
	}
	if !supportsSubChunkRequests(nil) {
		t.Error("unknown protocol should keep sub-chunk requests")
	}
}

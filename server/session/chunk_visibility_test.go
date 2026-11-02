package session

import (
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/df-mc/dragonfly/server/world"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol/login"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

type chunkVisibilityConn struct {
	cache bool
}

func (*chunkVisibilityConn) Close() error                                               { return nil }
func (*chunkVisibilityConn) IdentityData() login.IdentityData                           { return login.IdentityData{} }
func (*chunkVisibilityConn) ClientData() login.ClientData                               { return login.ClientData{} }
func (c *chunkVisibilityConn) ClientCacheEnabled() bool                                 { return c.cache }
func (*chunkVisibilityConn) Proto() minecraft.Protocol                                  { return nil }
func (*chunkVisibilityConn) ChunkRadius() int                                           { return 1 }
func (*chunkVisibilityConn) Latency() time.Duration                                     { return 0 }
func (*chunkVisibilityConn) Flush() error                                               { return nil }
func (*chunkVisibilityConn) RemoteAddr() net.Addr                                       { return nil }
func (*chunkVisibilityConn) ReadPacket() (packet.Packet, error)                         { return nil, io.EOF }
func (*chunkVisibilityConn) WritePacket(packet.Packet) error                            { return nil }
func (*chunkVisibilityConn) StartGameContext(context.Context, minecraft.GameData) error { return nil }

func TestChunkVisibleRequiresCurrentDeliveredChunk(t *testing.T) {
	w := world.Config{Synchronous: true}.New()
	other := world.Config{Synchronous: true}.New()
	t.Cleanup(func() {
		_ = other.Close()
		_ = w.Close()
	})
	loader := world.NewLoader(1, w, world.NopViewer{})
	s := &Session{chunkLoader: loader, conn: &chunkVisibilityConn{}, chunkTransactions: map[world.ChunkPos]map[uint64]struct{}{}}
	pos := world.ChunkPos{0, 0}
	if s.ChunkVisible(w, pos) {
		t.Fatal("chunk was visible before its loader delivered it")
	}
	if err := w.Do(func(tx *world.Tx) { loader.Load(tx, 1) }).Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	if !s.ChunkVisible(w, pos) {
		t.Fatal("delivered chunk was not visible")
	}
	if s.ChunkVisible(other, pos) {
		t.Fatal("chunk from another world was reported visible")
	}

	s.conn = &chunkVisibilityConn{cache: true}
	s.chunkTransactions[pos] = map[uint64]struct{}{42: {}}
	if s.ChunkVisible(w, pos) {
		t.Fatal("chunk was visible while its cache transaction was pending")
	}
	s.resolveChunkTransactions([]uint64{42})
	if !s.ChunkVisible(w, pos) {
		t.Fatal("resolved cache transaction did not make the chunk visible")
	}

	s.chunkTransactions[pos] = map[uint64]struct{}{84: {}}
	s.blobs = map[uint64][]byte{84: {1, 2, 3}}
	s.openChunkTransactions = []map[uint64]struct{}{{84: {}}}
	s.packets = make(chan outboundMessage, 1)
	s.closeBackground = make(chan struct{})
	if err := (&ClientCacheBlobStatusHandler{}).Handle(&packet.ClientCacheBlobStatus{MissHashes: []uint64{84}}, s, nil, nil); err != nil {
		t.Fatal(err)
	}
	message := <-s.packets
	if _, ok := message.packet.(*packet.ClientCacheMissResponse); !ok {
		t.Fatalf("first queued packet = %T, want *packet.ClientCacheMissResponse", message.packet)
	}
	if !s.ChunkVisible(w, pos) {
		t.Fatal("chunk did not become visible after its cache-miss response was queued")
	}
}

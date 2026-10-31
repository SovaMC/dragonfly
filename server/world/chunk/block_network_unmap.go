package chunk

import (
	"bytes"
	"fmt"

	"github.com/df-mc/dragonfly/server/block/cube"
)

// NetworkDecodeBlocks decodes only the block sub-chunks of a network chunk payload laid out for the payload range and
// places them into a Chunk with range r. Biomes and block entities that follow the sub-chunks are ignored, which
// allows payloads in historical formats to be read.
func NetworkDecodeBlocks(br BlockRegistry, data []byte, count int, payload, r cube.Range) (*Chunk, error) {
	src := New(br, payload)
	if count < 0 || count > len(src.sub) {
		return nil, fmt.Errorf("invalid sub-chunk count %d: chunk range has %d sub-chunks", count, len(src.sub))
	}
	buf := bytes.NewBuffer(data)
	for i := range count {
		index := uint8(i)
		sub, err := decodeSubChunk(buf, src, &index, NetworkEncoding)
		if err != nil {
			return nil, err
		}
		if int(index) >= len(src.sub) {
			return nil, fmt.Errorf("invalid sub-chunk index %d: chunk range has %d sub-chunks", index, len(src.sub))
		}
		src.sub[index] = sub
	}
	if payload == r {
		return src, nil
	}
	c := New(br, r)
	offset := (payload.Min() - r.Min()) >> 4
	for i, sub := range src.sub {
		if j := i + offset; j >= 0 && j < len(c.sub) {
			c.sub[j] = sub
		}
	}
	return c, nil
}

// ReplaceBlockRuntimeIDs replaces every block runtime ID in the palettes of the chunk with the value returned by f.
func (chunk *Chunk) ReplaceBlockRuntimeIDs(f func(uint32) uint32) {
	for _, sub := range chunk.sub {
		sub.ReplaceBlockRuntimeIDs(f)
	}
}

// ReplaceBlockRuntimeIDs replaces every block runtime ID in the palettes of the sub-chunk with the value returned by f.
func (sub *SubChunk) ReplaceBlockRuntimeIDs(f func(uint32) uint32) {
	if sub == nil {
		return
	}
	for _, storage := range sub.storages {
		if storage != nil {
			storage.palette.Replace(f)
		}
	}
}

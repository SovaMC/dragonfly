package chunk

import (
	"maps"
	"testing"
)

type derivedStateTestRegistry struct{ mappingTestBlockRegistry }

func (derivedStateTestRegistry) StateToRuntimeID(name string, props map[string]any) (uint32, bool) {
	return 42, name == "minecraft:oak_stairs" && props["minecraft:corner"] == "none"
}

type missingPropertyTestRegistry struct {
	mappingTestBlockRegistry
	defaults map[string]any
}

func (r missingPropertyTestRegistry) DefaultProperties(name string) (map[string]any, bool) {
	return r.defaults, name == "minecraft:fire"
}

func (r missingPropertyTestRegistry) StateToRuntimeID(name string, props map[string]any) (uint32, bool) {
	return 43, name == "minecraft:fire" && maps.Equal(props, r.defaults)
}

func TestMissingNativePropertiesRepairWithoutMutatingSharedState(t *testing.T) {
	defaults := map[string]any{"age": int32(4), "upper_block_bit": byte(0)}
	registry := missingPropertyTestRegistry{defaults: defaults}
	for _, complete := range []bool{false, true} {
		properties := map[string]any{"age": int32(4)}
		if complete {
			properties["upper_block_bit"] = byte(0)
		}
		before := maps.Clone(properties)
		for attempt := 0; attempt < 2; attempt++ {
			legacy := false
			rid, err := (BlockPaletteEncoding{Blocks: registry, legacy: &legacy}).DecodeBlockState(map[string]any{
				"name": "minecraft:fire", "version": CurrentBlockVersion, "states": properties,
			})
			if err != nil || rid != 43 {
				t.Fatalf("complete=%v attempt=%d: rid=%d err=%v", complete, attempt, rid, err)
			}
			if legacy == complete {
				t.Fatalf("complete=%v: repair=%v", complete, legacy)
			}
			if !maps.Equal(properties, before) || defaults["age"] != int32(4) || defaults["upper_block_bit"] != byte(0) {
				t.Fatal("decoding mutated input or shared defaults")
			}
		}
	}
}

func TestAddedDerivedPropertiesMarkExistingVersionForRepair(t *testing.T) {
	legacy := false
	properties := map[string]any{"weirdo_direction": int32(0), "upside_down_bit": byte(0)}
	_, err := (BlockPaletteEncoding{Blocks: derivedStateTestRegistry{}, legacy: &legacy}).DecodeBlockState(map[string]any{
		"name": "minecraft:oak_stairs", "version": CurrentBlockVersion, "states": properties,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !legacy {
		t.Fatal("new corner defaults must be derived from neighbours even when the stored block version is unchanged")
	}
	if _, mutated := properties["minecraft:corner"]; mutated {
		t.Fatal("state upgrade mutated input properties")
	}
}

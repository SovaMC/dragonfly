package session

import (
	"image/color"
	"testing"

	"github.com/df-mc/dragonfly/server/player/skin"
	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
)

func TestSkinProtocolRoundTripPreservesAppearanceMetadata(t *testing.T) {
	s := skin.New(64, 64)
	s.FullID = "full"
	s.Persona = true
	s.Premium = true
	s.PersonaCapeOnClassic = true
	s.CapeID = "cape"
	s.ArmSize = protocol.ArmSizeWide
	s.SkinColour = color.RGBA{R: 1, G: 2, B: 3, A: 0xff}
	s.AnimationData = []byte("animation")
	s.GeometryDataEngineVersion = []byte("engine")
	s.ProfileHash = "profile"
	s.PersonaPieces = []protocol.PersonaPiece{{PieceID: "piece", PieceType: protocol.PieceTypeBody, PackID: uuid.New(), ProductID: "product"}}
	s.PieceTintColours = []protocol.PersonaPieceTintColour{{PieceType: "persona_body", Colours: [4]color.RGBA{{R: 1, A: 0xff}}}}

	wire := skinToProtocol(s)
	if !wire.PersonaSkin || !wire.PremiumSkin || !wire.PersonaCapeOnClassicSkin || wire.ArmSize != protocol.ArmSizeWide {
		t.Fatalf("wire persona flags = %+v", wire)
	}
	decoded, err := protocolToSkin(wire)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.FullID != s.FullID || decoded.CapeID != s.CapeID || decoded.ProfileHash != s.ProfileHash {
		t.Fatalf("decoded identifiers = %+v", decoded)
	}
	if len(decoded.PersonaPieces) != 1 || decoded.PersonaPieces[0] != s.PersonaPieces[0] {
		t.Fatalf("decoded persona pieces = %#v", decoded.PersonaPieces)
	}
	if len(decoded.PieceTintColours) != 1 || decoded.PieceTintColours[0] != s.PieceTintColours[0] {
		t.Fatalf("decoded persona tints = %#v", decoded.PieceTintColours)
	}
	if string(decoded.AnimationData) != "animation" || string(decoded.GeometryDataEngineVersion) != "engine" {
		t.Fatalf("decoded opaque metadata was not preserved")
	}
}

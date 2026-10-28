package session

import (
	"slices"
	"sync"

	"github.com/df-mc/dragonfly/server/internal/sliceutil"
	"github.com/df-mc/dragonfly/server/player/skin"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/google/uuid"
	"github.com/sandertv/gophertunnel/minecraft/protocol"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

var sessions = new(sessionList)

type sessionList struct {
	mu sync.Mutex
	s  []*Session
}

func (l *sessionList) Add(s *Session) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for _, other := range l.s {
		// Show all sessions to the new session and the new session to all
		// existing sessions.
		l.sendSessionTo(s, other)
		l.sendSessionTo(other, s)
	}
	// Show the new session to itself.
	l.sendSessionTo(s, s)
	l.s = append(l.s, s)
}

func (l *sessionList) Remove(s *Session, entity world.Entity) {
	l.mu.Lock()
	removedFrom := slices.Clone(l.s)
	for _, other := range l.s {
		l.unsendSessionFrom(s, other)
	}
	l.s = sliceutil.DeleteVal(l.s, s)
	l.mu.Unlock()

	if entity == nil {
		return
	}
	for _, other := range removedFrom {
		if other.viewLayer != nil {
			other.viewLayer.Remove(entity)
		}
	}
}

func (l *sessionList) Lookup(id uuid.UUID) (*Session, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	// A reconnect can arrive after the old session leaves Server's online map
	// but before its world and player-list cleanup finishes. Prefer the newest.
	for i := len(l.s) - 1; i >= 0; i-- {
		if l.s[i].ent.UUID() == id {
			return l.s[i], true
		}
	}
	return nil, false
}

func (l *sessionList) sendSessionTo(s, to *Session) {
	if s != to && s.ent.UUID() == to.ent.UUID() {
		// Never replace a connection's own actor with its overlapping session.
		return
	}
	runtimeID := uint64(selfEntityRuntimeID)

	to.entityMutex.Lock()
	defer to.entityMutex.Unlock()
	if to.listedPlayers == nil {
		to.listedPlayers = make(map[uuid.UUID]*world.EntityHandle)
	}
	if previous := to.listedPlayers[s.ent.UUID()]; previous != nil && previous != s.ent {
		to.removeListedPlayer(previous)
	}
	if s != to {
		if existing, ok := to.entityRuntimeIDs[s.ent]; ok {
			runtimeID = existing
		} else {
			to.currentEntityRuntimeID += 1
			runtimeID = to.currentEntityRuntimeID
		}
	}
	to.listedPlayers[s.ent.UUID()] = s.ent
	to.entityRuntimeIDs[s.ent] = runtimeID
	to.entities[runtimeID] = s.ent

	to.writePacket(&packet.PlayerList{
		Entries: []protocol.PlayerListEntry{{
			ActionType:     protocol.PlayerListActionAdd,
			UUID:           s.ent.UUID(),
			EntityUniqueID: int64(runtimeID),
			Username:       s.conn.IdentityData().DisplayName,
			XUID:           s.conn.IdentityData().XUID,
			PlayFabID:      s.conn.IdentityData().PlayFabID,
			BuildPlatform:  int32(protocol.DeviceUnknown),
			Skin:           skinToProtocol(s.joinSkin),
		}},
	})
}

func (l *sessionList) unsendSessionFrom(s, from *Session) {
	from.entityMutex.Lock()
	defer from.entityMutex.Unlock()
	if from.listedPlayers[s.ent.UUID()] != s.ent {
		return
	}
	from.removeListedPlayer(s.ent)
}

// removeListedPlayer retires the actor before its UUID and runtime ID can be
// reused. entityMutex protects both the ownership change and packet ordering.
func (s *Session) removeListedPlayer(handle *world.EntityHandle) {
	id := s.entityRuntimeIDs[handle]
	if _, shown := s.shownEntities[handle]; shown && handle != s.ent {
		s.writePacket(&packet.RemoveActor{EntityUniqueID: int64(id)})
	}
	delete(s.shownEntities, handle)
	delete(s.pendingPlayers, handle)
	delete(s.playerDimensions, id)
	delete(s.entities, id)
	delete(s.entityRuntimeIDs, handle)
	delete(s.listedPlayers, handle.UUID())

	s.writePacket(&packet.PlayerList{
		Entries: []protocol.PlayerListEntry{{
			ActionType: protocol.PlayerListActionRemove,
			UUID:       handle.UUID(),
		}},
	})
}

// skinToProtocol converts a skin to its protocol representation.
func skinToProtocol(s skin.Skin) protocol.Skin {
	var animations []protocol.SkinAnimation
	for _, animation := range s.Animations {
		protocolAnim := protocol.SkinAnimation{
			ImageWidth:  uint32(animation.Bounds().Max.X),
			ImageHeight: uint32(animation.Bounds().Max.Y),
			ImageData:   animation.Pix,
			FrameCount:  float32(animation.FrameCount),
		}
		switch animation.Type() {
		case skin.AnimationHead:
			protocolAnim.AnimationType = protocol.SkinAnimationHead
		case skin.AnimationBody32x32:
			protocolAnim.AnimationType = protocol.SkinAnimationBody32x32
		case skin.AnimationBody128x128:
			protocolAnim.AnimationType = protocol.SkinAnimationBody128x128
		}
		protocolAnim.ExpressionType = uint32(animation.AnimationExpression)
		animations = append(animations, protocolAnim)
	}

	fullID := s.FullID
	if fullID == "" {
		fullID = uuid.New().String()
	}
	model := s.Model
	if len(model) == 0 {
		model = []byte("{}")
	}
	capeID := s.CapeID
	if capeID == "" {
		capeID = uuid.New().String()
	}
	geometryVersion := s.GeometryDataEngineVersion
	if len(geometryVersion) == 0 {
		geometryVersion = []byte(protocol.CurrentVersion)
	}
	return protocol.Skin{
		SkinID:                    uuid.New().String(),
		SkinResourcePatch:         s.ModelConfig.Encode(),
		SkinImageWidth:            uint32(s.Bounds().Max.X),
		SkinImageHeight:           uint32(s.Bounds().Max.Y),
		SkinData:                  s.Pix,
		CapeImageWidth:            uint32(s.Cape.Bounds().Max.X),
		CapeImageHeight:           uint32(s.Cape.Bounds().Max.Y),
		CapeData:                  s.Cape.Pix,
		SkinGeometry:              model,
		AnimationData:             slices.Clone(s.AnimationData),
		PersonaSkin:               s.Persona,
		PremiumSkin:               s.Premium,
		PersonaCapeOnClassicSkin:  s.PersonaCapeOnClassic,
		CapeID:                    capeID,
		FullID:                    fullID,
		ArmSize:                   s.ArmSize,
		SkinColour:                s.SkinColour,
		PersonaPieces:             slices.Clone(s.PersonaPieces),
		PieceTintColours:          slices.Clone(s.PieceTintColours),
		Animations:                animations,
		Trusted:                   true,
		OverrideAppearance:        true,
		GeometryDataEngineVersion: slices.Clone(geometryVersion),
		ProfileHash:               s.ProfileHash,
	}
}

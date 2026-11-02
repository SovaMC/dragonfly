package server

import (
	"log/slog"
	"net"
	"testing"

	"github.com/df-mc/dragonfly/server/player"
	"github.com/df-mc/dragonfly/server/session"
	"github.com/df-mc/dragonfly/server/world"
	"github.com/go-gl/mathgl/mgl64"
	"github.com/google/uuid"
	"github.com/pelletier/go-toml"
	"github.com/sandertv/gophertunnel/minecraft"
	"github.com/sandertv/gophertunnel/minecraft/protocol/login"
	"github.com/sandertv/gophertunnel/minecraft/protocol/packet"
)

func TestSwimmingUserConfigDefaultAndOverride(t *testing.T) {
	if DefaultConfig().Players.DisableSwimming {
		t.Fatal("default config disabled swimming")
	}
	for _, input := range []struct {
		toml string
		want bool
	}{
		{"", false},
		{"[Players]\nDisableSwimming = false\n", false},
		{"[Players]\nDisableSwimming = true\n", true},
	} {
		var user UserConfig
		if err := toml.Unmarshal([]byte(input.toml), &user); err != nil {
			t.Fatal(err)
		}
		user.Resources.Folder = t.TempDir()
		conf, err := user.Config(slog.Default())
		if err != nil {
			t.Fatal(err)
		}
		if conf.DisableSwimming != input.want {
			t.Fatalf("DisableSwimming=%t for %q, want %t", conf.DisableSwimming, input.toml, input.want)
		}
	}
}

type swimmingConfigConn struct{ session.Conn }

func (swimmingConfigConn) IdentityData() login.IdentityData { return login.IdentityData{} }
func (swimmingConfigConn) ClientData() login.ClientData     { return login.ClientData{} }
func (swimmingConfigConn) RemoteAddr() net.Addr             { return &net.UDPAddr{} }
func (swimmingConfigConn) Proto() minecraft.Protocol        { return nil }
func (swimmingConfigConn) ChunkRadius() int                 { return 1 }
func (swimmingConfigConn) WritePacket(packet.Packet) error  { return nil }
func (swimmingConfigConn) Close() error                     { return nil }

func TestSwimmingNewConnectionsApplyDefaultsBeforeSpawn(t *testing.T) {
	w := world.Config{Synchronous: true}.New()
	t.Cleanup(func() { _ = w.Close() })
	for _, serverDisabled := range []bool{false, true} {
		for _, playerDisabled := range []bool{false, true} {
			srv := &Server{
				conf: Config{Log: slog.Default(), MaxChunkRadius: 1, DisableSwimming: serverDisabled},
				p:    make(map[uuid.UUID]*onlinePlayer),
			}
			id := uuid.New()
			incoming := srv.createPlayer(id, swimmingConfigConn{}, player.Config{
				Position: mgl64.Vec3{0.5, 64, 0.5}, DisableSwimming: playerDisabled,
			}, w)
			srv.p[id] = incoming.p
			if err := w.Do(func(tx *world.Tx) {
				p := tx.AddEntity(incoming.p.handle).(*player.Player)
				defer func() {
					incoming.s.Close(nil, p)
					incoming.s.CloseConnection()
					_ = tx.RemoveEntity(p).Close()
				}()
				want := !serverDisabled && !playerDisabled
				p.StartSwimming()
				if p.SwimmingEnabled() != want || p.Swimming() != want {
					t.Fatalf("server disabled=%t, player disabled=%t: enabled=%t, swimming=%t", serverDisabled, playerDisabled, p.SwimmingEnabled(), p.Swimming())
				}
				p.SetSwimmingEnabled(true)
				p.StartSwimming()
				if !p.Swimming() {
					t.Fatal("gameplay could not override the connection default")
				}
			}).Wait(t.Context()); err != nil {
				t.Fatal(err)
			}
		}
	}
}

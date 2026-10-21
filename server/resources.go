package server

import (
	"errors"

	"github.com/sandertv/gophertunnel/minecraft/resource"
)

var errResourceReloadUnsupported = errors.New("no listener supports resource pack reloading")

// ResourceReloadingListener is an optional Listener capability for replacing
// the resource packs offered to new connections. Listener wrappers must forward
// ReloadResources to preserve this capability; embedding only Listener hides it.
type ResourceReloadingListener interface {
	Listener
	// ReloadResources replaces the offered packs without changing negotiations
	// already in progress. Implementations must retain their own slice snapshot
	// and allow concurrent connections to read it safely. Packs must not be
	// modified after publication. A nil or empty slice clears the offered packs.
	ReloadResources([]*resource.Pack)
}

// ReloadResources replaces the resource packs offered to new connections by
// every listener that supports runtime resource pack replacement. Existing
// connections are not affected and must reconnect to negotiate the new packs.
// Concurrent calls are serialised across all supporting listeners. Connections
// negotiating during a reload may receive either the previous or the new packs.
func (srv *Server) ReloadResources(resources []*resource.Pack) error {
	srv.resourceReloadMu.Lock()
	defer srv.resourceReloadMu.Unlock()

	reloaded := false
	for _, l := range srv.listeners {
		if resourceListener, ok := l.(ResourceReloadingListener); ok {
			resourceListener.ReloadResources(resources)
			reloaded = true
		}
	}
	if !reloaded {
		return errResourceReloadUnsupported
	}
	return nil
}

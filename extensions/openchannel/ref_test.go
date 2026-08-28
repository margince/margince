// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

import (
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

// A minted ref must be one the CORE will route. The edge answers 404 to a path
// segment outside its published grammar, before this unit's handler is reached
// at all — so a ref that failed the core's own predicate would be a URL handed
// to a member that silently never works, on an endpoint that looks open.
//
// It is checked against extension.ValidInboundRef and not against a copy of the
// grammar: a copy that admitted one character more would mint exactly those
// URLs.
func TestAMintedAddressIsOneTheInboundEdgeWillRoute(t *testing.T) {
	t.Parallel()
	seen := map[string]bool{}
	for range 64 {
		ref, err := newEndpointRef()
		if err != nil {
			t.Fatalf("minting: %v", err)
		}
		if !extension.ValidInboundRef(ref) {
			t.Fatalf("minted %q, which the inbound edge would refuse before the unit is reached", ref)
		}
		if seen[ref] {
			t.Fatalf("minted %q twice in 64 draws, so two members would share one address", ref)
		}
		seen[ref] = true
	}
}

// It is an ADDRESS and not a credential, which the code says in three places.
// What a test can hold is the property behind those sentences: a ref is the
// bounded, loggable kind of value rather than the key-sized kind, and it stays
// inside the core's bound as the encoding is changed underneath it.
func TestAMintedAddressIsShortEnoughToReadOutLoud(t *testing.T) {
	t.Parallel()
	ref, err := newEndpointRef()
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	// base64url of refBytes, unpadded: ceil(refBytes*8/6).
	if want := (refBytes*8 + 5) / 6; len(ref) != want {
		t.Fatalf("minted a %d-character address, want %d — the encoding changed under the bound", len(ref), want)
	}
	if len(ref) > extension.MaxInboundRef {
		t.Fatalf("minted a %d-character address, over the core's %d bound", len(ref), extension.MaxInboundRef)
	}
}

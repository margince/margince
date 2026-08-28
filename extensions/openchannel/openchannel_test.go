// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

import (
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

// The declaration spells its secret key and its slug as LITERALS, because the
// operator manifest is derived from New's AST without compiling it. The rest of
// the unit reaches for constants. These tests are what stop the two spellings
// from becoming two values — a mismatch on the key mounts an edge whose secret
// nothing ever wrote, and a mismatch on the slug opens an endpoint at a path no
// request arrives on. Both fail silently: the endpoint exists and refuses
// everything, which looks exactly like a sender with the wrong secret.
func TestTheDeclaredSecretKeyIsTheOneTheHandlersRead(t *testing.T) {
	t.Parallel()
	declared := New().Secrets
	if len(declared) != 1 {
		t.Fatalf("the unit declares %d secrets; this test knows about one", len(declared))
	}
	if declared[0].Key != inboundSecretKey {
		t.Fatalf("the declaration names %q and the handlers read %q", declared[0].Key, inboundSecretKey)
	}
	if declared[0].Scope != extension.SecretScopeUser {
		t.Fatalf("the signing secret is declared at %q scope; it is one member's own", declared[0].Scope)
	}
}

func TestTheDeclaredSlugIsTheOneAnOpenedEndpointClaims(t *testing.T) {
	t.Parallel()
	edges := New().Inbound
	if len(edges) != 1 {
		t.Fatalf("the unit declares %d inbound edges; this test knows about one", len(edges))
	}
	if edges[0].Slug != inboundSlug {
		t.Fatalf("the declaration mounts %q and an opened endpoint claims %q", edges[0].Slug, inboundSlug)
	}
	if edges[0].Secret != inboundSecretKey {
		t.Fatalf("the edge verifies against %q and the handlers seal under %q", edges[0].Secret, inboundSecretKey)
	}
	// The same grammar the manifest generator and the boot preflight apply, so
	// a declaration this unit could not mount fails here rather than at boot.
	if err := edges[0].Validate(); err != nil {
		t.Fatalf("the declared edge is not mountable: %v", err)
	}
}

// Every tool the declaration serves must be one the contract fragment declares,
// which the core refuses at boot. What this holds is the half the core cannot
// see: that the unit ships a handler for each of them rather than a name.
func TestEveryDeclaredToolCarriesAHandler(t *testing.T) {
	t.Parallel()
	tools := New().Tools
	if len(tools) == 0 {
		t.Fatal("the unit declares no tools, so its whole governed surface is unreachable")
	}
	seen := map[string]bool{}
	for _, tool := range tools {
		if tool.Handle == nil {
			t.Fatalf("tool %q declares no handler", tool.Name)
		}
		if seen[tool.Name] {
			t.Fatalf("tool %q is declared twice", tool.Name)
		}
		seen[tool.Name] = true
	}
}

// The unit's SQL layer has to be IN the binary. Shipping the directory without
// wiring the embed passes every other gate and then boots against a database
// where the tables were never created.
func TestTheMigrationsLayerIsEmbedded(t *testing.T) {
	t.Parallel()
	layer := New().Migrations
	if layer == nil {
		t.Fatal("the declaration carries no migrations layer")
	}
	for _, name := range []string{
		"migrations/0001_endpoint.up.sql",
		"migrations/0001_endpoint.down.sql",
		"migrations/0002_inbound.up.sql",
		"migrations/0002_inbound.down.sql",
	} {
		if _, err := layer.Open(name); err != nil {
			t.Fatalf("%s is not in the embedded layer: %v", name, err)
		}
	}
}

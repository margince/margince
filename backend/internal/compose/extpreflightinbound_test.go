// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/pkg/extension"
)

func inboundEndpointFixture(slug, secret string) extension.InboundEndpoint {
	return extension.InboundEndpoint{
		Slug:    slug,
		Secret:  secret,
		MaxBody: 64 << 10,
		Rate: extension.InboundRate{
			PerIP:       extension.Rate{Limit: 60, Window: time.Minute},
			PerEndpoint: extension.Rate{Limit: 120, Window: time.Minute},
		},
		Skew: 5 * time.Minute,
		Handle: func(context.Context, extension.Runtime, extension.InboundRequest) (extension.InboundOutcome, error) {
			return extension.InboundAccepted, nil
		},
	}
}

// unitWithInbound is a unit declaring one user-scoped secret and the given
// endpoints against it.
func unitWithInbound(name string, endpoints ...extension.InboundEndpoint) extension.Extension {
	return extension.Extension{
		Name:    extension.Name(name),
		Version: "0.1.0",
		Secrets: []extension.SecretsRequest{{Key: "inbound", Scope: extension.SecretScopeUser}},
		Inbound: endpoints,
	}
}

func TestPreflightInboundAdmitsADeclaredEndpoint(t *testing.T) {
	if err := preflightInbound(unitWithInbound("u", inboundEndpointFixture("capture", "inbound"))); err != nil {
		t.Fatalf("a whole declaration was refused: %v", err)
	}
}

// A unit declaring no edge is not degraded — it is every unit that existed
// before this field.
func TestPreflightInboundAdmitsAUnitWithNoEdge(t *testing.T) {
	if err := preflightInbound(extension.Extension{Name: "u", Version: "0.1.0"}); err != nil {
		t.Fatalf("a unit with no inbound edge was refused: %v", err)
	}
}

func TestPreflightInboundRefusesASlugDeclaredTwice(t *testing.T) {
	e := unitWithInbound("u",
		inboundEndpointFixture("capture", "inbound"),
		inboundEndpointFixture("capture", "inbound"),
	)
	err := preflightInbound(e)
	if err == nil {
		t.Fatal("boot admitted the same slug twice")
	}
	if !strings.Contains(err.Error(), "twice") {
		t.Fatalf("refusal said %q, which does not say the slug was declared twice", err)
	}
}

// The one that would otherwise mount, serve, and refuse every request it ever
// received — indistinguishable from a sender holding the wrong key.
func TestPreflightInboundRefusesAnUndeclaredSecret(t *testing.T) {
	e := unitWithInbound("u", inboundEndpointFixture("capture", "typo"))
	err := preflightInbound(e)
	if err == nil {
		t.Fatal("boot admitted an endpoint verifying against a secret the unit never declared")
	}
	for _, want := range []string{"typo", "does not declare", "wrong key"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("refusal said %q, which does not carry %q", err, want)
		}
	}
}

// A WORKSPACE-scoped secret is the installation's own credential. Verifying one
// member's counterparty against it would let any member's sender sign for any
// other member's endpoint.
func TestPreflightInboundRefusesAWorkspaceScopedSecret(t *testing.T) {
	e := unitWithInbound("u", inboundEndpointFixture("capture", "shared"))
	e.Secrets = []extension.SecretsRequest{{Key: "shared", Scope: extension.SecretScopeWorkspace}}
	if err := preflightInbound(e); err == nil {
		t.Fatal("boot admitted an endpoint verifying against a workspace-scoped secret")
	}
}

// The published grammar runs at boot too, so a declaration that reached the
// composed set outside the generator path is judged the same way.
func TestPreflightInboundRunsThePublishedGrammar(t *testing.T) {
	e := unitWithInbound("u", inboundEndpointFixture("Capture", "inbound"))
	err := preflightInbound(e)
	if err == nil {
		t.Fatal("boot admitted a slug the published grammar refuses")
	}
	if !strings.Contains(err.Error(), "is not a slug") {
		t.Fatalf("refusal said %q, which is not the published grammar's", err)
	}
}

func TestReserveInboundMountsRefusesTwoUnitsClaimingOnePath(t *testing.T) {
	mounts := map[string]extension.Name{}
	first := unitWithInbound("u", inboundEndpointFixture("capture", "inbound"))
	if err := reserveInboundMounts(first, mounts); err != nil {
		t.Fatalf("the first claim was refused: %v", err)
	}
	// Same unit name and same slug is the only way the derived path collides,
	// which the namespace reservation already precludes — held here anyway,
	// because the mount is derived from the pair and this check must not rest
	// on another one happening to make the pair unique.
	err := reserveInboundMounts(first, mounts)
	if err == nil {
		t.Fatal("two units claimed one mounted path")
	}
	if !strings.Contains(err.Error(), "/webhooks/ext/u/capture") {
		t.Fatalf("refusal said %q, which does not name the contested path", err)
	}
}

func TestReserveInboundMountsAdmitsDistinctPaths(t *testing.T) {
	mounts := map[string]extension.Name{}
	for _, e := range []extension.Extension{
		unitWithInbound("u", inboundEndpointFixture("capture", "inbound")),
		unitWithInbound("v", inboundEndpointFixture("capture", "inbound")),
	} {
		if err := reserveInboundMounts(e, mounts); err != nil {
			t.Fatalf("distinct units were refused one another's path: %v", err)
		}
	}
	if len(mounts) != 2 {
		t.Fatalf("reserved %d paths, want 2", len(mounts))
	}
}

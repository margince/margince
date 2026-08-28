// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

import (
	"embed"
	"time"

	"github.com/margince/margince/backend/pkg/extension"
)

// migrations carries the unit's SQL layer INTO the binary. Shipping the
// directory without setting the field below passes every gate — the SQL
// blessed, the catalog checked — and then boots against a database where the
// tables were never created.
//
//go:embed migrations
var migrations embed.FS

// New returns the unit's declaration: inert data, holding no handle into the
// core. Every field is a literal, because the operator manifest is derived from
// this function's AST without compiling it.
func New() extension.Extension {
	return extension.Extension{
		Name:    "openchannel",
		Version: "1.0.0",
		Tools: []extension.Tool{
			{Name: "openchannel_open", Handle: open},
			{Name: "openchannel_mint_secret", Handle: mintSecret},
			{Name: "openchannel_set_enabled", Handle: setEnabled},
			{Name: "openchannel_register_url", Handle: registerURL},
			{Name: "openchannel_list_inbound", Handle: listInbound},
		},
		// User scope, and it is the whole authority story for the anonymous
		// edge: the secret an arriving request is verified against is the
		// OWNER's, so a request that verifies is one that member agreed to
		// receive. Boot refuses an inbound endpoint naming a secret the unit
		// did not declare, so this entry is what makes the edge mountable at
		// all rather than a nicety for the manifest.
		Secrets: []extension.SecretsRequest{
			{Key: "inbound", Scope: extension.SecretScopeUser},
		},
		// The anonymous edge, and the numbers are what this unit ASKS for: an
		// installation ceiling may grant less, and the manifest then records
		// both. Every one of them bounds what a party with no session can make
		// this installation spend before its signature has even been checked.
		Inbound: []extension.InboundEndpoint{{
			Slug:   "receive",
			Secret: "inbound",
			// 64 KiB. A channel message and its envelope, with room for a
			// generous one — and far under the 1 MiB published ceiling, which
			// is sized for surfaces that take uploads. This edge takes none:
			// a connector that needed to carry a file would carry a reference
			// to one, because the alternative is letting a stranger choose how
			// much this installation reads per request.
			MaxBody: 64 << 10,
			Rate: extension.InboundRate{
				// The per-IP bucket is the tighter of the two on purpose: one
				// sender is expected from one address, and a flood is not.
				PerIP: extension.Rate{Limit: 60, Window: time.Minute},
				// Two a second sustained across every source, which is a busy
				// channel rather than a quiet one — and it is the bucket that
				// still holds when a flood is spread across many addresses.
				PerEndpoint: extension.Rate{Limit: 120, Window: time.Minute},
			},
			// Five minutes, well under the published ceiling. It is the drift
			// an unattended sender's clock accumulates between synchronisations;
			// wider than that and a captured request stays replayable for
			// longer than it takes to notice one was captured.
			Skew:   5 * time.Minute,
			Handle: receive,
		}},
		Migrations: migrations,
	}
}

// inboundSecretKey is the declared secret above, as the code that reaches for
// it names it. The declaration holds a LITERAL because the manifest is derived
// statically, without compiling this unit, so a constant there would be a name
// the generator cannot resolve. The two are the same string on purpose, and a
// mismatch mounts an edge whose secret nothing ever wrote.
//
// Held by: TestTheDeclaredSecretKeyIsTheOneTheHandlersRead (extensions/openchannel/openchannel_test.go)
const inboundSecretKey = "inbound"

// inboundSlug names the declared edge above, as the code that reaches for it
// names it: the last segment of the public path senders POST to, the value an
// opened endpoint claims, and what the handler resolves back to an owner. It is
// a literal there for the reason inboundSecretKey is, and a mismatch would open
// an endpoint at a path nothing ever arrives on.
//
// Held by: TestTheDeclaredSlugIsTheOneAnOpenedEndpointClaims (extensions/openchannel/openchannel_test.go)
const inboundSlug = "receive"

// The unit's tables, schema-qualified. Every statement writes them through
// these constants: the ext schema is on no search_path the app connects with,
// so an unqualified name would resolve to a public table this unit does not
// own.
//
// The LEDGER names the endpoint table without the schema (endpointEntity),
// because audit_log.entity_type names a kind of record rather than a path to
// one. endpointTable is that name with the schema put in front of it, so a
// rename is one edit and reaches both readings.
const (
	endpointEntity = "ext_openchannel_endpoint"
	endpointTable  = "ext." + endpointEntity
	inboundTable   = "ext.ext_openchannel_inbound"
)

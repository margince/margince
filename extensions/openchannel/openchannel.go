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

// migrationsLayer names the directory the directive above carries. The
// directive takes a path and not a constant, so the name is written twice on
// purpose; what walks the pair is the embed test, which reads this directory
// off disk and opens every file it finds out of the embedded copy.
const migrationsLayer = "migrations"

// New returns the unit's declaration: inert data, holding no handle into the
// core. Every field is a literal, because the operator manifest is derived from
// this function's AST without compiling it.
func New() extension.Extension {
	return extension.Extension{
		Name:        "openchannel",
		Version:     "1.0.0",
		Description: "An anonymous, signed endpoint an outside party can post to, with its own records and a job that drains arrivals into the CRM.",
		Tools: []extension.Tool{
			{Name: "openchannel_open", Handle: open},
			{Name: "openchannel_read_endpoint", Handle: readEndpoint},
			{Name: "openchannel_mint_secret", Handle: mintSecret},
			{Name: "openchannel_set_enabled", Handle: setEnabled},
			{Name: "openchannel_register_url", Handle: registerURL},
			{Name: "openchannel_list_inbound", Handle: listInbound},
			{Name: "openchannel_list_outbound", Handle: listOutbound},
		},
		// The declaration that lets this unit reach core capture at all, and the
		// source every request it drains is attributed to. A record naming
		// anything else is refused at the call rather than landed under an
		// invented provenance namespace.
		//
		// The email merge key is declared because an arriving document names both
		// ends of the message, and the address on the far end is what lets a
		// counterparty already known from mail be recognised as the same human
		// rather than quietly becoming a second contact. It is VOUCHED FOR only
		// as far as the sender's own signature goes — which is the honest bound,
		// and why a message carrying an account id is resolved by that instead.
		Ingress: []extension.IngressSource{
			{
				System: "openchannel",
				Lands:  []extension.RecordKind{extension.KindActivity},
				Merges: []extension.MergeKey{extension.MergeKeyEmail},
			},
		},
		// The transport this unit supplies. A message it carries lands as kind
		// `message` with `openchannel` on the provider column — the unit names the
		// TRANSPORT and never the kind, which is what keeps the two axes separate
		// from outside the core.
		//
		// Live is required because Send is present: a transport that can transmit
		// must be able to say whether it still may, or the core has to guess at
		// the one moment guessing is unrecoverable.
		//
		// A literal rather than the `provider` constant, for the reason the
		// ingress source above is one: the manifest is derived STATICALLY,
		// without compiling the unit, so a constant here would be a name the
		// generator cannot resolve. The two are the same string on purpose and a
		// test holds them equal.
		Channels: []extension.Channel{
			{Provider: "openchannel", Send: send, Live: live},
		},
		Jobs: []extension.Job{
			{Name: "drain", Handle: drain},
		},
		// A landed request's timeline entry can be archived by a person, and
		// nothing in the core knows this unit's queue claims to have produced it.
		// Without this subscription that claim stays true forever about an entry
		// nobody can see.
		Subscriptions: []extension.Subscription{
			{Name: "withdraw_captured", Events: []string{"activity.archived"}, Handle: withdrawCaptured},
		},
		// The ways the drain fails, in this unit's own words (failureclasses.go).
		// Declaring them is what lets an operator reading a dead job see that the
		// capture pipeline was unreachable rather than that the failure could not
		// be classified — and the list is named rather than inlined because the
		// same values are what the drain returns and what the queue rows record,
		// so the declared set and the recorded class cannot become two sets.
		FailureClasses: failureClasses,
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
// names it: the second-to-last segment of the public path senders POST to, and
// the edge each opened endpoint belongs to. It is a literal there for the reason
// inboundSecretKey is, and a mismatch would key every endpoint to an edge no
// request arrives on.
//
// It is not what an ARRIVING request is resolved by — it is the same for every
// member. That is the minted ref's job (ref.go).
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
	inboundEntity  = "ext_openchannel_inbound"
	inboundTable   = "ext." + inboundEntity
	outboundTable  = "ext.ext_openchannel_outbound"
)

// Where a received request is in the queue.
//
// They are the CHECK constraint's own vocabulary and the contract's published
// enum, which is why they are constants rather than literals at each call site:
// three spellings of one word is how a drain comes to look for rows in a state
// nothing ever writes.
//
// `pending` is waiting to be acted on; `ingested` has been landed on the
// timeline; `failed` is one the drain has stopped attempting, which stays visible
// rather than being deleted; and `withdrawn` is one whose timeline entry has
// since been archived.
const (
	stateWaiting   = "pending"
	stateLanded    = "ingested"
	stateParked    = "failed"
	stateWithdrawn = "withdrawn"
)

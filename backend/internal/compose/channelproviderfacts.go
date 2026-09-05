// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What `GET /v1/channel-providers` publishes about a transport, and where those
// facts come from (ADR-0107/A158).
//
// They live in the composition root because that is the only layer that knows
// what this binary COMPOSED. A module cannot answer it — `internal/modules` may
// not enumerate connectors, and `channel_provider` carries no workspace_id to
// scope a query by, so a module has no unscoped pool to read it from either.

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/pkg/extension"
)

// credentialWorkspaceBot is the shape a CORE connector's credential takes: one
// bot for the installation. It is the published vocabulary's own value rather
// than a second spelling of it — a unit declares from the same two constants,
// and a registry that agreed with the enum only by coincidence would drift the
// first time either moved.
const credentialWorkspaceBot = string(extension.CredentialWorkspaceBot)

// transportCore and transportUnit are who SUPPLIES a transport: a connector
// compiled into the core, or an extension unit under extensions/. Closed for
// credentialWorkspaceBot's reason — it describes the shape of the supply, not
// which providers exist.
const (
	transportCore = "core"
	transportUnit = "unit"
)

// channelProviderFacts is one transport as the discovery endpoint publishes it.
type channelProviderFacts struct {
	provider string
	// transport is who supplies it. It is a fact about the INSTALLATION rather
	// than about the provider: `telegram` is a core transport here and could be
	// a unit's somewhere else, which is why the collision between the two is a
	// boot failure rather than a naming convention.
	transport         string
	label             string
	credentialModel   string
	suppliesTransport bool
	// carriage is what this transport can carry alongside a message. The ZERO
	// value — carries nothing — for a provider with no composed sender, and for
	// one whose sender never declared carriage: the directory publishes the
	// no-default rule rather than an absence a client would have to interpret.
	carriage connector.Carriage
}

// coreProviderLabels names the transports this binary compiles in, for the
// cases where title-casing the id gets the name wrong.
//
// A map rather than an optional interface on the connector port: exactly one
// provider needs it today (WhatsApp, whose title-cased id reads "Whatsapp"),
// and a port method with one implementer is an abstraction ahead of its second
// caller. When a unit declares its own channel in the slice that gives it one,
// the label arrives with the declaration and this map stays what it says it is
// — the CORE names.
var coreProviderLabels = map[string]string{
	"whatsapp": "WhatsApp",
}

// providerLabel is what a human reads where the raw id would otherwise appear.
//
// It is derived or compiled in, never operator-configured, because the endpoint
// is readable by every authenticated seat: a provider id plus a display name is
// not privileged, but anything an administrator typed might be.
func providerLabel(provider string) string {
	if known, ok := coreProviderLabels[provider]; ok {
		return known
	}
	return titleCasedID(provider)
}

// titleCasedID turns an id into a name, treating `_` and `-` as word breaks so
// `deal_room` reads "Deal Room" rather than "Deal_room".
//
// BOTH separators, one function, because the two ids that need naming here use
// one each: a provider id is snake (`zalo_oa`, channel_provider's own CHECK
// `^[a-z][a-z0-9_]*$`) and an ingress system key is kebab (`zalo-oa`,
// IngressSource.System's grammar). A provider id cannot contain `-` at all, so
// the second break is inert on that path.
//
// An EMPTY segment stays a segment, which is the whole reason this is a Split
// and not a Fields: `initcap(replace(provider,'_',' '))` is the label migration
// 0247 seeds a fresh installation with, and it renders `deal__room` as
// "Deal  Room" — two spaces. Collapsing the run here would make a fresh install
// and a booted one disagree about that provider's name, which is exactly what
// TestTheSeededLabelMatchesTheOneBootWrites promises cannot happen, and the
// double underscore is legal under the column's own CHECK.
func titleCasedID(id string) string {
	// `-` folded onto `_` rather than split on both: one separator keeps the
	// empty-segment behaviour above, and no id in either grammar carries both.
	words := strings.Split(strings.ReplaceAll(id, "-", "_"), "_")
	for i, w := range words {
		if w == "" {
			continue
		}
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}

// channelProviderFactsFor describes every provider the registry knows, marking
// the ones that can actually carry a message.
//
// registered is every row the registry holds; sending maps the subset the binary
// composed a MessageSender for to what that sender can carry. The difference is
// the honest case this endpoint exists to publish: whatsapp is registered so a
// hand-logged WhatsApp message can name what carried it, and nothing composes
// it, so it supplies no transport until A103's connector lands.
//
// sending is a MAP rather than a second name list because "does it send" and
// "what can it carry" are one answer — presence says it sends, the value says
// what it carries — and two collections would be two places a provider could be
// in one and missing from the other.
//
// credential_model is workspace_bot for every core connector, because a core
// channel connector binds ONE bot for the installation. A unit declares its own;
// see unitChannelFacts.
func channelProviderFactsFor(registered []string, sending map[string]connector.Carriage) []channelProviderFacts {
	out := make([]channelProviderFacts, 0, len(registered))
	for _, p := range registered {
		carriage, sends := sending[p]
		out = append(out, channelProviderFacts{
			provider:          p,
			transport:         transportCore,
			label:             providerLabel(p),
			credentialModel:   credentialWorkspaceBot,
			suppliesTransport: sends,
			carriage:          carriage,
		})
	}
	return out
}

// unitChannelFacts describes every transport this boot's UNITS supply, and
// holds the one rule a unit's declaration cannot check for itself: it may not
// shadow a core connector.
//
// THE COLLISION IS THE SHARPEST FAILURE THIS SURFACE HAS, which is why it is a
// boot failure and not a warning. A unit declaring `telegram` would take over
// the row the workspace's own bot is registered under, and every Telegram reply
// a rep wrote would then leave on the unit's per-member credential instead —
// the same message, sent by a different person, with nothing on the screen
// different. Refusing at boot costs an installation a rename.
//
// It lives HERE rather than in preflightChannels because this is the first
// point at which both sets exist: the core's transports are decided when the
// capture registry is built, which can happen after extension registration, so
// the preflight would answer from an empty set and pass the collision it exists
// to catch. The unit-vs-unit half is still the preflight's — that one needs no
// core knowledge, and refusing it earlier is a better error.
//
// WHAT COUNTS AS A CORE TRANSPORT is the REGISTRY's answer, not the composed
// sender list's, and the difference is a hole this check shipped with once.
// `capture.Registry.ChannelProviders` returns only connectors that implement
// the message seam — `telegram` and nothing else — while `channel_provider`
// carries every reserved core name, `whatsapp` among them: registered by
// migration so a hand-logged WhatsApp message can say what carried it, with no
// Go connector behind it. Checked against the composed list alone, a unit
// declaring `whatsapp` passed, the upsert re-pointed the core row at the unit,
// and every previously-unrepliable WhatsApp conversation in the installation
// became one the unit transmits.
//
// credential_model is the unit's OWN declaration, carried through unchanged.
//
// It used to be derived — per_member for everything under extensions/, on the
// reasoning that a unit holds one sealed secret per member. That is true of a
// personal-account transport and false of a company one, and the tier permits
// both: an Official Account is a shared business account that happens to ship as
// a unit. Deriving it answered wrongly for that whole class, and wrongly in the
// direction nothing detects — a message narrowed onto the mailbox path keeps
// exactly one reader, the connecting admin, so no orphan invariant fires while a
// company's customer correspondence has become one person's private mail.
//
// Channel.Validate refuses a unit that declares neither, so there is no default
// to be wrong about here.
//
// The label is DERIVED from the id rather than declared, which is the same
// decision providerLabel documents: this endpoint is readable by every
// authenticated seat, and a derived name cannot carry text somebody typed.
func unitChannelFacts(reserved map[string]bool) ([]channelProviderFacts, error) {
	var out []channelProviderFacts
	for _, ext := range ComposedExtensions() {
		for _, ch := range ext.Channels {
			if !ch.CredentialModel.Valid() {
				// The registry column has its own CHECK, so an undeclared model
				// is refused either way — but by name, three layers down, at the
				// upsert. Refusing here names the unit, the transport and the
				// two answers, which is what a unit author needs and what a
				// constraint name cannot give them.
				return nil, fmt.Errorf("compose: extension %q declares the transport %q without a credential model — say %q if one credential serves the whole installation, %q if each member deposits their own; it decides whether this transport's messages are the company's correspondence or one person's",
					ext.Name, ch.Provider, extension.CredentialWorkspaceBot, extension.CredentialPerMember)
			}
			if reserved[ch.Provider] {
				return nil, fmt.Errorf("compose: extension %q declares the transport %q, which this installation reserves for the core — a message on it would leave on the unit's credential instead of the workspace's, so rename the unit's channel", ext.Name, ch.Provider)
			}
			out = append(out, channelProviderFacts{
				provider:          ch.Provider,
				transport:         transportUnit,
				label:             providerLabel(ch.Provider),
				credentialModel:   string(ch.CredentialModel),
				suppliesTransport: ch.SuppliesTransport(),
				// The zero descriptor: extension.Channel has no field for a
				// unit to declare carriage on yet. sendableCarriage states why
				// that is the no-default rule holding rather than a gap.
				carriage: connector.Carriage{},
			})
		}
	}
	return out, nil
}

// captureSourceFacts is one capture provenance id as the directory publishes
// it: the id itself, and what a human reads instead of it.
type captureSourceFacts struct {
	source string
	label  string
}

// captureSourceFactsFor describes every provenance id the composed UNITS land
// records under — `ext:<unit>:<system>`, one per declared ingress source.
//
// IT IS NOT A TRANSPORT LIST, and that difference is why these facts exist apart
// from channelProviderFacts. A transport is something a message can leave on;
// this answers "which connector carried the message this row came from" for a
// record that arrived on no channel at all — a unit's mail, note or call, whose
// trace carries the natural key's source system (traceConnector, in
// capture/sinktrace.go) rather than a provider. That id cannot BE a transport:
// it fails
// channel_provider.provider's grammar, no reply resolves against it, and
// registering it would mint a send anchor for a transport nobody supplies.
//
// So the directory publishes it as what it is. Without that, a member reading
// the capture trace of a unit's own records sees the raw `ext:` form beside the
// same unit's channel messages reading "Dispact" — one transport under two
// spellings, one of them provenance nobody outside this repository can parse.
//
// Every declared source is published whether or not the unit also supplies a
// channel: a capture-only unit is the case with no transport entry standing
// beside it, so it is the one that most needs naming.
//
// THE LABEL GOES THROUGH providerLabel, not straight to titleCasedID, and the
// difference is only ever visible on a name the core already spells for itself:
// a unit declaring ingress from `whatsapp` would otherwise publish "Whatsapp" in
// the same document where `data` carries the core's "WhatsApp". Two spellings of
// one transport in one list is the defect this whole seam exists to remove, so
// the compiled-in spelling wins wherever there is one. A kebab system key simply
// misses that map and falls through, which is why one call covers both.
//
// A system key that MATCHES a core transport is not refused, unlike a unit's
// channel declaration (unitChannelFacts): the two claims are different. A
// channel says "I transmit on this", and a unit claiming a core transport there
// would re-point the core's row at the unit's own credential. An ingress source
// says "I bring records in from this", and a unit that ingests Telegram exports
// naming it `telegram` is telling the truth — refusing it would break the honest
// case to prevent a reviewed first-party unit from lying about what it captures.
// The shared label is then correct rather than a collision: the record really did
// travel on that transport, and the reader should not have to know whether it
// arrived on a channel or through a unit's ingress.
//
// The label is derived from the DECLARED system key and never
// operator-configured, for the reason providerLabel documents.
func captureSourceFactsFor(exts []extension.Extension) []captureSourceFacts {
	var out []captureSourceFacts
	for _, unit := range exts {
		for _, src := range unit.Ingress {
			out = append(out, captureSourceFacts{
				source: extSourceSystem(string(unit.Name), src.System),
				label:  providerLabel(src.System),
			})
		}
	}
	return out
}

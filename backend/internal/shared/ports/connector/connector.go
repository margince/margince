// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package connector defines the capture/integration seam (interfaces.md
// §1): the uniform interface every integration implements — Gmail,
// calendar, telephony, the scrape/enrichment connector, and the deepest
// one, an incumbent SoR adapter. A connector normalizes provider records
// and hands them to the Sink; the capture module (never the connector) writes the
// row, the audit entry, and the domain event, so RBAC/RLS/audit stay in
// one place.
package connector

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// Connector is the seam every integration implements, registered in the
// connector registry by Descriptor().Name.
type Connector interface {
	// Descriptor is static metadata, read at registration; it drives scope
	// enforcement, the 🟢/🟡 tier, crm gen, and the contract.
	Descriptor() Descriptor

	// Authenticate establishes or refreshes credentials for one
	// per-user, per-workspace connection and returns the opaque persisted
	// Auth the other methods reuse.
	Authenticate(ctx context.Context, req AuthRequest) (Auth, error)

	// Sync pulls INCREMENTALLY from cursor (history API / delta token /
	// updatedAt watermark), emits normalized records via the Sink, and
	// returns the advanced cursor. Idempotent: writes key on
	// (source_system, source_id) so the DB unique index dedupes replays.
	Sync(ctx context.Context, auth Auth, cursor Cursor, sink Sink) (Cursor, error)

	// Normalize maps ONE raw provider record to provenance-stamped domain
	// records. Pure — no I/O — so the mapping is the agent-edited,
	// test-guarded surface. Returns an ErrSkip-wrapped error for
	// deliberately excluded input (personal-mail rule etc.).
	Normalize(ctx context.Context, raw RawRecord) ([]NormalizedRecord, error)

	// HealthCheck feeds the ops surface; an outage degrades capture but
	// never blocks core CRM (capture is async on the job queue).
	HealthCheck(ctx context.Context, auth Auth) error
}

// Watcher is the OPTIONAL push-watch seam for a provider whose change
// notifications come through a subscription that lapses (Gmail Pub/Sub's 7 days,
// Graph's ≤3). Separate from Connector because the one-shot IMAP puller has no
// such subscription; the renewal scan type-asserts and skips a non-Watcher.
type Watcher interface {
	// Watch registers (or, on a repeat call, renews) the provider push
	// subscription against topic and returns the watermark to resume from plus
	// the new expiration deadline. It performs provider I/O like Sync; it never
	// touches the CRM or the connection row (the registry persists the result).
	Watch(ctx context.Context, auth Auth, topic string) (WatchResult, error)
}

// WatchResult is the outcome of registering or renewing a provider push watch:
// the historyId/delta anchor and when the watch expires. The registry stores
// ExpiresAt in capture_connection.watch_expires_at, which the renewal scan
// keys on.
type WatchResult struct {
	HistoryID string
	ExpiresAt time.Time
	// The connector's OPAQUE HANDLE on the subscription it just made, handed
	// back to RenewWatch so a renewal addresses that subscription rather than
	// going looking for one. The contract says nothing about its FORM: this
	// column reaches every database reader and every backup, so a handle holds
	// what a renewal needs and nothing more — for Graph the subscription id, and
	// never a URL carrying the token that admits a notification.
	//
	// Empty where the provider has no handle: a Gmail watch is addressed by the
	// mailbox and the topic. The registry stores it and CLEARS it when the
	// connection is rebound, because the mailbox it named is gone.
	Ref string
}

// WatchRenewer renews a push subscription BY THE HANDLE the provider gave it.
// Optional and type-asserted like Watcher; the registry falls back to Watch,
// which is also the first registration's path.
//
// Separate rather than a wider Watch because the questions differ: Watch says
// "make sure there is a subscription" and RenewWatch says "extend THIS one",
// which is the question a renewal actually has.
type WatchRenewer interface {
	// RenewWatch extends the subscription named by ref and returns its new
	// deadline. A ref the provider no longer knows is not an error the caller
	// has to handle specially: the implementation registers a fresh
	// subscription and returns its handle, which is what the caller wanted.
	RenewWatch(ctx context.Context, auth Auth, topic, ref string) (WatchResult, error)
}

// AccountLabeler names the account an Auth bundle belongs to — the mailbox
// address, for display only. Optional and type-asserted, exactly like Watcher
// and Backfiller: the Connector interface stays frozen, and a connector that
// cannot name its account simply does not implement this.
//
// The label is never an identifier: nothing routes, authorizes or deduplicates
// on it. capture_connection is keyed (user_id, provider).
type AccountLabeler interface {
	AccountLabel(auth Auth) (string, error)
}

// GrantedScoper reports the PROVIDER scopes a connection actually holds — the
// provider's own vocabulary ("Mail.Read"), read back from the Auth bundle the
// consent sealed. Distinct from Descriptor.Scopes, which is this system's
// internal permission vocabulary; the two never share storage.
//
// Optional and type-asserted like AccountLabeler: a connector that cannot know
// its granted scopes (a direct-credential one, with no consent step) simply
// does not implement it, and the connection records no claim rather than a
// false one.
type GrantedScoper interface {
	GrantedScopes(auth Auth) ([]string, error)
}

// Descriptor — declared capabilities; ⊆ the granting human's scopes.
type Descriptor struct {
	// Name is the stable id: "gmail", "gcal", "imap". Lower-case letters,
	// digits and underscores, starting with a letter — the shape the contract
	// publishes as ProviderRef, held by ValidName at registration.
	Name     string
	Version  string
	Scopes   []principal.Scope
	RiskTier mcp.RiskTier // capture/read = auto_execute; any outbound = confirmation_required
	Tools    []mcp.ToolSpec
	Produces []datasource.EntityType
}

// AuthRequest carries whatever the provider handshake needs (OAuth code,
// API key); shape is provider-specific and opaque to the registry.
type AuthRequest struct {
	WorkspaceConnection string
	Payload             []byte
}

// Sink is how a connector hands normalized records to the CRM for
// upsert + provenance + event emit.
type Sink interface {
	// Upsert writes one record idempotently by its NaturalKey, stamps
	// provenance, writes the audit row, and emits the domain event.
	Upsert(ctx context.Context, rec NormalizedRecord) (datasource.EntityRef, error)
}

// MeetingCanceller is the Sink's second calendar verb: a meeting already
// captured is called off. It is separate from Sink rather than a method on it
// because cancelling is a calendar-only act — the mail and channel connectors
// have nothing to say through it, and widening the one interface every
// connector and fixture implements would make them all answer a question only
// two of them are asked.
//
// A connector that finds no captured meeting under the key has nothing to
// cancel, which is the ordinary case: most cancelled events were never worth
// capturing in the first place. That is reported as a no-op, never an error.
type MeetingCanceller interface {
	// CancelMeeting marks the meeting captured under this natural key as
	// cancelled. Idempotent: cancelling an already-cancelled meeting writes
	// nothing and succeeds, so a resynced calendar costs no second transition.
	CancelMeeting(ctx context.Context, key NaturalKey, at time.Time) error
}

// MessageRemover is the mail connectors' equivalent: the provider reports that
// a message this workspace captured is gone from the mailbox it came from.
//
// Separate from Sink for MeetingCanceller's reason — only the mail connectors
// have anything to say through it, and a calendar or channel connector should
// not have to answer a question it is never asked.
//
// What the removal MEANS is not this seam's to decide. A connector reports that
// the provider said a message is gone; whether the captured copy follows is a
// question about colleagues' claims and statutory duties that the implementor
// answers. A connector that decided it would be deciding for mailboxes it
// cannot see.
type MessageRemover interface {
	// RemoveMessage reports that the message captured under this natural key
	// was deleted at the provider, by the owner of the mailbox it arrived in.
	//
	// Whose mailbox comes from the principal on ctx, the way every other write
	// a sync makes resolves its seat — a connector that passed an owner would
	// be naming a seat it learned from an address, which is the thing capture
	// spends its whole identity spine not doing.
	//
	// Idempotent and forgiving: a key naming nothing this workspace captured is
	// the ordinary case, not an error — most deleted mail was never captured.
	RemoveMessage(ctx context.Context, key NaturalKey) error
}

// NormalizedRecord — a provider record mapped onto the clean relational
// core with provenance. Fields holds the typed domain struct for
// EntityType so a wrong mapping fails to compile, not at runtime.
type NormalizedRecord struct {
	EntityType datasource.EntityType
	NaturalKey NaturalKey
	Fields     any
	Links      []datasource.EntityRef
	Source     string // "<system>:<id>" — REQUIRED
	CapturedBy string // "connector:<name>" — REQUIRED
	Raw        []byte // re-parseable original → raw jsonb, off the hot path

	// StoredOriginal names the raw_capture row this record was read from.
	//
	// A connector that persisted the original ITSELF — a poll that has to
	// store the update before it can queue the work — fills it, and hands over
	// no Raw. Every other lane leaves it zero and the sink fills it from the
	// row it stores, so by the time the activity is written it is set wherever
	// an original exists at all.
	//
	// It is durable because the two writers of raw_capture key it differently:
	// the sink by the domain natural key, a connector's own store by the
	// provider's REDELIVERY key. Those agree for mail and for nothing else, so
	// a purge that matched on them found a channel message's original never.
	StoredOriginal ids.UUID

	// What this record is known by to EVERY door, as distinct from the natural
	// key, which is only what THIS provider called it. Empty when the provider
	// states none. Mail needs none — its natural key already IS the shared
	// identity — but one MEETING on two calendars carries two provider event ids
	// and needs the iCal UID plus occurrence to resolve as one.
	//
	// Filled only from a value the PROVIDER stated, never synthesized: a made-up
	// identity collides two unrelated records, which is worse than the duplicate
	// it set out to prevent. It carries the PARTS rather than a finished key,
	// because the key's spelling belongs to the module owning `activity_identity`
	// and a connector may not import it.
	CrossDoorIdentity CrossDoorIdentity

	// The address the RECEIVING infrastructure recorded delivery to, empty when
	// no such claim could be trusted. It is how a forwarding alias is
	// discoverable at all — an alias is never the From of anything the mailbox
	// sends. Filled only from a header position a sender could not have authored
	// (mailmap.TopDeliveredTo); empty is the safe answer and the common one.
	DeliveredTo string

	// The provider's own filing places, each qualified by its namespace:
	// "gmail:<labelId>", "graph:<folderId>", "imap:<mailbox>". Qualified rather
	// than bare because one contact may have connected two mailboxes on
	// different providers, and an unqualified "inbox" would be one rule matching
	// two unrelated places. A message can sit in several.
	//
	// NOT folded to lower case: a Graph folder id is base64url and an IMAP
	// mailbox is case-sensitive except INBOX. Only the provider prefix is fixed.
	Containers []string

	// Counterparty is the human on the other side of a captured message —
	// the auto-create pipeline's input (ADR-0063). Zero for records that
	// carry no counterparty (a lead import, a system activity); the
	// resolver never runs for those.
	Counterparty Counterparty

	// The conversation identity, and what counts as "the conversation" differs by
	// shape. Mail roots on a MESSAGE id — the References root, else In-Reply-To,
	// else its own — never a provider's private conversation id, which joins
	// nothing here; a fresh message roots at its own Message-ID so a later reply
	// joins from the first message onward. A channel has no such chain: the chat
	// IS the conversation, so it is the provider's chat id
	// ("telegram:<bot_id>:<chat_id>"), the one case where that is the right join
	// key. Empty only for a mail record with none of its three sources.
	ThreadKey string

	// The FURTHER parties beyond the mailbox owner and Counterparty — CCs, a
	// meeting's attendees and organizer. Additive rather than a replacement,
	// because direction is defined against the two ENDS of the exchange and
	// everyone else is present without being either.
	//
	// Addresses, not records: resolving one is capture's job at stamping time,
	// and one that resolves to nobody is still kept — an attendee without a
	// record is a fact about the meeting.
	Participants []MessageParticipant

	// Reports that the PROVIDER enumerated Participants, so binding one to a
	// colleague's user_id records what the provider stated rather than what a
	// sender typed. The mail and calendar answers differ: a recipient list on
	// inbound mail is the sender's own text, so an outsider could mail a synced
	// mailbox with `Cc: ceo@ourcompany.com` and manufacture an interaction edge,
	// while a calendar attendee list is the provider's own record read back over
	// an authenticated connection.
	//
	// Unexported with one setter, so a record crossing a serialization boundary
	// arrives un-attested rather than wrongly attested; the zero value refuses
	// the binding.
	participantsAreProviderAttested bool

	// Addresses is EVERY address this record names, including the connected
	// owner's own. The internal-vs-external decision is taken over it, which is
	// why it overlaps Counterparty and Participants rather than complementing
	// them: those are the derived ENDS, and a message is internal only when
	// every party to it is.
	//
	// None means "I cannot enumerate the parties", not "there are none" — an
	// unenumerable message is never treated as internal, so it is captured.
	Addresses []string

	// The files this record carried, already bounded, renamed safely and typed by
	// their bytes. A connector never enforces those rules itself: they belong to
	// the one parser every mail adapter shares, so a new adapter cannot arrive
	// without them.
	Parts []Part

	// PartDrops names the files the bounds refused. It is carried rather than
	// discarded so a message whose attachments were too many or too large is
	// distinguishable from a message that had none — silence would report the
	// two identically, and only one of them means something is missing.
	PartDrops []PartDrop
}

// WithProviderAttestedParticipants returns a copy recording that the PROVIDER
// enumerated this record's participants — a calendar's own attendee list, read
// back over an authenticated connection, rather than a recipient header some
// sender wrote.
//
// attested is the connector core's answer about the SOURCE, never anything the
// record itself claimed: a record that names its own kind "meeting" attests
// nothing by saying so, which is why the extension ingress (which copies a
// third-party unit's kind straight through) leaves this false and keeps the
// mail rule.
//
// A caller that attests nothing leaves the answer false, which refuses the
// colleague binding rather than trusting it.
func (r NormalizedRecord) WithProviderAttestedParticipants(attested bool) NormalizedRecord {
	r.participantsAreProviderAttested = attested
	return r
}

// ParticipantsAreProviderAttested reports whether the provider itself
// enumerated Participants, which is what lets capture bind a colleague's
// user_id from the list.
func (r NormalizedRecord) ParticipantsAreProviderAttested() bool {
	return r.participantsAreProviderAttested
}

// CrossDoorIdentity is what a record is known by to every door, in its parts.
// Only meetings carry one: an occurrence is a SERIES plus the meeting within it,
// because a recurring event shares one iCal UID across all of them. The parts
// travel unjoined so exactly one function — capture's, through the seam compose
// injects — writes every key, and the two doors cannot spell one meeting twice.
type CrossDoorIdentity struct {
	// The provider-stated identity of the recurring event, or of a one-off.
	// Empty yields no cross-door identity at all.
	Series string
	// Which meeting within the series, as its own start. Zero likewise yields no
	// identity: a series without one names every meeting in it at once.
	Occurrence time.Time
}

// Stated reports whether the provider actually gave both parts. A partial
// identity is no identity: keying on the series alone would resolve every
// occurrence of a weekly call to the first one.
func (c CrossDoorIdentity) Stated() bool {
	return strings.TrimSpace(c.Series) != "" && !c.Occurrence.IsZero()
}

// NaturalKey is the (source_system, source_id) idempotency key the DB
// unique indexes enforce (data-model §7/§8).
type NaturalKey struct {
	SourceSystem string
	SourceID     string

	// Reports that SourceID embeds the provider's identifier for a HUMAN rather
	// than naming a message, event or notification, which decides whether the
	// pipeline trace stores the id or a hash of it. Declared HERE because the
	// producer is the only party that knows: inferring it from how the record
	// named its counterparty answers a different question, and a connector
	// keying on notification ids while naming counterparties two ways had half
	// its traces hashed and half not.
	//
	// False is the ordinary case and the zero value on purpose: a natural key is
	// almost always a message id, which identifies a message rather than a
	// contact and is what makes a support question answerable.
	SourceIDNamesAContact bool
}

// EmailSourceSystem is the natural-key SOURCE SYSTEM every transport carrying an
// RFC822 message writes: one message is one activity whether it arrived over
// Gmail, Graph or IMAP, sent or received. The transport is not lost — it stays
// in `source`, `captured_by` and capture_connection.provider — it just stops
// being part of the answer to "is this the same message", because one message
// reaching a workspace over two connectors is one message.
//
// Identity is exactly equality of the parsed Message-ID, which the mail parser
// already requires. Nothing is fuzzy-matched: two Message-IDs stay two
// activities however alike they read, and a forged one gets the collision this
// key has always had.
//
// A caller-facing create path may not write it: only a connector that
// authenticated as one, or our own send, may claim a mail identity.
const EmailSourceSystem = "email"

// ExtensionSourceSystemPrefix namespaces every record an extension unit lands,
// so a unit can never spell a core connector's identity. Spelled here because
// compose WRITES it onto a unit's records and capture's admit check READS it.
const ExtensionSourceSystemPrefix = "ext:"

type (
	Cursor    []byte // opaque incremental-sync watermark
	Auth      []byte // opaque persisted credential bundle
	RawRecord []byte // one provider record as received
)

// ErrSkip marks a record a connector intentionally skipped (excluded or
// out of scope); the sync loop counts it, never surfaces it as a failure.
var ErrSkip = errors.New("connector: record intentionally skipped")

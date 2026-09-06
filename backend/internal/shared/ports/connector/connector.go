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
	"time"

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

// Watcher is the OPTIONAL push-watch seam a connector implements when its
// provider delivers change notifications through a subscription that must be
// renewed before it lapses (Gmail Pub/Sub's 7-day watch, Graph's ≤3-day
// subscription). It is separate from Connector because a provider without a
// renewable push subscription (the one-shot IMAP puller) does not implement it;
// the registry's watch-renewal scan type-asserts for it and skips a connector
// that is not a Watcher.
type Watcher interface {
	// Watch registers (or, on a repeat call, renews) the provider push
	// subscription against topic and returns the watermark to resume from plus
	// the new expiration deadline. It performs provider I/O like Sync; it never
	// touches the CRM or the connection row (the registry persists the result).
	Watch(ctx context.Context, auth Auth, topic string) (WatchResult, error)
}

// WatchResult is the outcome of registering/renewing a provider push watch:
// the historyId/delta anchor at watch time and when the watch expires. The
// registry stores ExpiresAt in capture_connection.watch_expires_at, which the
// renewal scan keys on (CAP-DDL-2, idx_capture_watch_renew).
type WatchResult struct {
	HistoryID string
	ExpiresAt time.Time
	// Ref is the connector's OPAQUE HANDLE on the subscription it just made.
	// Stored verbatim in capture_connection.watch_ref and handed back to
	// WatchRenewer.RenewWatch, so a renewal addresses the subscription it made
	// rather than going looking for it.
	//
	// Opaque to everything outside the connector that minted it, which is what
	// lets a connector decide for itself what a renewal needs to hold. It is
	// also the reason the contract says nothing about the FORM: for Graph it is
	// the subscription id and nothing else, because whether that subscription
	// still points at the endpoint being renewed is a question Microsoft
	// answers, and a stored answer would be a copy of a URL carrying the token
	// that admits a notification. This column reaches every database reader and
	// every backup, so what a handle does not have to hold, it does not hold.
	//
	// Empty where the provider has no such handle: a Gmail watch is addressed by
	// the mailbox and the topic, and there is nothing to remember.
	//
	// The registry's side of the contract is to store it and to CLEAR it when
	// the connection is rebound to a different account — a handle names a
	// subscription in the mailbox it was made against, and that mailbox is gone.
	Ref string
}

// WatchRenewer renews a push subscription BY THE HANDLE the provider gave it.
//
// Optional and type-asserted, exactly like Watcher: a provider whose watch is
// addressed by the mailbox alone has no handle to renew by and does not
// implement this. The registry falls back to Watch, which is also what happens
// on the first registration and for a connection made before any handle was
// stored.
//
// Separate from Watcher rather than a wider Watch, because the two questions are
// different: Watch says "make sure there is a subscription", and answering it
// without a handle means finding one first. RenewWatch says "extend THIS one",
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

	// DeliveredTo is the address the RECEIVING infrastructure recorded this
	// message as delivered to, and empty whenever no such claim could be
	// trusted. It is how a forwarding alias is discoverable at all: an alias
	// is never the From of anything the mailbox sends, so the send side cannot
	// see it.
	//
	// A connector fills this only from a header position a sender could not
	// have authored — mailmap.TopDeliveredTo is that decision, made once. An
	// empty value is the safe answer and the common one; a connector that
	// cannot make the judgement leaves it empty rather than guessing.
	DeliveredTo string

	// Counterparty is the human on the other side of a captured message —
	// the auto-create pipeline's input (ADR-0063). Zero for records that
	// carry no counterparty (a lead import, a system activity); the
	// resolver never runs for those.
	Counterparty Counterparty

	// ThreadKey is the conversation identity, and what counts as "the
	// conversation" differs by shape. Mail roots on a MESSAGE id — the
	// References root, else In-Reply-To, else the message's own id — never a
	// provider's private conversation id, a different namespace joining
	// nothing here (see SendReceipt); a fresh message with no reply headers
	// roots at its OWN Message-ID rather than empty, so a later reply joins
	// from the first message onward. A channel (Telegram) has no message-id
	// chain to root on — the chat itself IS the conversation, so ThreadKey is
	// the provider's chat id ("telegram:<bot_id>:<chat_id>"), the one case
	// where a provider conversation id is the right join key. Both feed the
	// same CAP-FORMULA-1 reply join and activity.thread_key. Empty only for a
	// mail record with none of its three sources; a channel record always has one.
	ThreadKey string

	// Participants are the FURTHER parties to this message beyond the mailbox
	// owner and Counterparty — the CCs on a thread, the attendees and organizer
	// of a meeting. A connector that reports none behaves exactly as before,
	// which is why this is additive rather than a replacement for Counterparty:
	// the two ends of the exchange are what direction is defined against, and
	// everyone else is present without being either end.
	//
	// They are addresses, not records. Resolving one to a colleague or a known
	// contact is capture's job at stamping time, and an address that resolves to
	// neither is still kept — an attendee nobody has a record for is a fact
	// about the meeting.
	Participants []MessageParticipant

	// participantsAreProviderAttested reports that the provider itself
	// enumerated Participants, so binding one to a colleague's user_id records
	// what the provider stated rather than what a sender typed.
	//
	// It exists because the mail answer and the calendar answer differ. A
	// recipient list on inbound MAIL is the sender's own text: nothing
	// authenticates it, so capture refuses to bind a user_id from one, or an
	// outsider could mail a synced mailbox with `Cc: ceo@ourcompany.com` and
	// manufacture an interaction edge. A calendar attendee list is not text on a
	// message — it is the provider's own record of who was invited, read back
	// over the authenticated connection of a seat that is on the event.
	//
	// Unexported with one setter, like Counterparty's own attestation, so a
	// caller cannot set it by assignment and a record crossing a serialization
	// boundary arrives un-attested rather than wrongly attested. The zero value
	// refuses the binding, so a connector that attests nothing keeps the mail
	// rule it has today.
	participantsAreProviderAttested bool

	// Addresses is EVERY address this record names — for mail the union of
	// From, To, Cc and whatever Bcc survived; for calendar the organizer and
	// attendees — including the connected owner's own. It is what the
	// internal-vs-external decision is taken over (ADR-0082/A127, formulas
	// §20), which is why it overlaps Counterparty and Participants rather than
	// complementing them: those two are the derived ENDS of the exchange, and
	// a message is internal only when every party to it is.
	//
	// A connector that reports none is saying "I cannot enumerate the parties",
	// not "there are none" — and an unenumerable message is never treated as
	// internal, so it is captured.
	Addresses []string

	// Parts are the files this record carried, already bounded, renamed safely
	// and typed by their bytes. A connector never enforces those rules itself:
	// they belong to the one parser every mail adapter shares, so a new adapter
	// cannot arrive without them.
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

// NaturalKey is the (source_system, source_id) idempotency key the DB
// unique indexes enforce (data-model §7/§8).
type NaturalKey struct {
	SourceSystem string
	SourceID     string

	// SourceIDNamesAPerson reports that SourceID embeds the provider's
	// identifier for a HUMAN — a chat id that is the customer's own account id,
	// say — rather than naming a message, an event or a notification.
	//
	// It decides whether the pipeline trace stores the id or a hash of it, and
	// it is declared HERE because the producer is the only party that knows.
	// The trace used to infer it from how the record named its counterparty,
	// which is a different question with a different answer: a connector whose
	// key is a notification id on every branch, and which names some
	// counterparties by a channel account and others by an address, had half
	// its traces hashed and half not (issue #1465). Neither half was wrong
	// about privacy and both were wrong about the key.
	//
	// False is the ordinary case and the zero value on purpose: a message id is
	// what a natural key almost always is, and it is what ADR-0082 §1 permits a
	// trace to record — it identifies a message rather than a person, and it is
	// what makes a support question answerable.
	SourceIDNamesAPerson bool
}

// EmailSourceSystem is the natural-key SOURCE SYSTEM every transport that
// carries an RFC822 message writes: one message is one activity whether it
// arrived over Gmail, Graph or IMAP, and whether we sent it or received it.
//
// The transport is NOT lost, it just stops being part of the identity. It
// stays in `source` (`gmail:<message-id>`) and in `captured_by`
// (`connector:gmail:<user>`), which is where every provenance reader already
// looks, and it stays in capture_connection.provider, which is what actually
// names a mailbox. What the transport must never be again is part of the
// answer to "is this the same message", because the same RFC822 message
// reaching one workspace over two connectors is one message and used to be two
// timeline rows.
//
// Identity here is exactly equality of the parsed Message-ID, which the mail
// parser has already required — a message without one is skipped before any
// write. Nothing is fuzzy-matched: two different Message-IDs stay two
// activities however alike they read, and a sender who reuses or forges one
// gets the collision this key has always had, now spanning the mail providers
// rather than sitting inside each.
//
// A caller-facing create path may not write it (activities' create mapper
// refuses it): only a connector that authenticated as one, or our own send,
// may claim a mail identity.
const EmailSourceSystem = "email"

// ExtensionSourceSystemPrefix namespaces the natural key of every record an
// extension unit lands, so a unit can never spell a core connector's identity
// — including the shared mail one above. Spelled here because two callers need
// the same string for opposite reasons: compose WRITES it onto a unit's
// records, and capture's admit check READS it to tell a unit's mail apart from
// a core connector's.
const ExtensionSourceSystemPrefix = "ext:"

type (
	Cursor    []byte // opaque incremental-sync watermark
	Auth      []byte // opaque persisted credential bundle
	RawRecord []byte // one provider record as received
)

// ErrSkip marks a record a connector intentionally skipped (excluded or
// out of scope); the sync loop counts it, never surfaces it as a failure.
var ErrSkip = errors.New("connector: record intentionally skipped")

// Backfiller is the OPTIONAL bounded-backfill seam (ADR-0063): a connector
// implements it when its provider can enumerate a mailbox backward from a
// date boundary. Like Watcher, it is separate from Connector so a provider
// without a date-bounded listing simply is not a Backfiller; the backfill
// engine type-asserts and refuses honestly. Backfill paging is disjoint from
// Sync's cursor by construction — incremental moves forward from the
// connect-time watermark while backfill pages backward on its own token, and
// the capture key makes any overlap a no-op.
type Backfiller interface {
	// EstimateBackfill returns the provider-side message count newer than
	// after — the scope shown before anything spends (the preview op's
	// number). An estimate, labeled as such; providers round.
	EstimateBackfill(ctx context.Context, auth Auth, after time.Time) (int, error)

	// BackfillPage pulls ONE bounded page of messages newer than after,
	// emitting each through the Sink. It performs provider I/O like Sync;
	// the engine persists cursor and counters from the returned result.
	BackfillPage(ctx context.Context, auth Auth, after time.Time, pageToken string, sink Sink) (BackfillPageResult, error)
}

// BackfillPageResult is one page's outcome: the token for the next page
// ("" = the window is exhausted) and the page's tally.
type BackfillPageResult struct {
	NextToken string
	Scanned   int
	Captured  int
	Skipped   int
}

// BackfillProgress carries a page's tally WHILE the page runs, so the engine
// can show progress that moves per message instead of once per committed
// page. A page is a hundred messages and minutes of provider I/O; without
// this the activation view sits at zero for the whole first page and reads
// as a dead import.
//
// Optional on both sides. The engine installs a reporter with
// WithBackfillProgress; a connector that never calls it reports only the
// BackfillPageResult it already returned, and behaves exactly as before.
// What a reporter records is advisory and transient — the page's own commit
// remains the one authority on a run's counters.
type BackfillProgress interface {
	// Observed reports THIS page's tally so far — the same three counts the
	// page's result carries, so a caller reading them mid-page still finds
	// scanned - captured = skipped. The numbers are absolute since the page
	// began, never deltas: a reporter that misses a call is corrected by the
	// next one instead of drifting, and a retried page restates rather than
	// double-counts.
	Observed(ctx context.Context, scanned, captured, skipped int)
}

// backfillProgressKey is the private context key — unexported and typed, so
// the reporter is reachable only through the two helpers below, never by
// another package reaching into the context for it directly.
type backfillProgressKey struct{}

// WithBackfillProgress installs the reporter a running page reports into.
// The engine calls this for the page it is about to run; nothing else should.
func WithBackfillProgress(ctx context.Context, p BackfillProgress) context.Context {
	return context.WithValue(ctx, backfillProgressKey{}, p)
}

// BackfillReporter is the value a connector reports through. It wraps the
// installed reporter, if any, so an unreported page costs a branch instead of
// a nil check at every call site.
type BackfillReporter struct{ to BackfillProgress }

// Observed forwards the page's tally, or discards it when nothing is
// listening.
func (r BackfillReporter) Observed(ctx context.Context, scanned, captured, skipped int) {
	if r.to != nil {
		r.to.Observed(ctx, scanned, captured, skipped)
	}
}

// BackfillProgressFrom returns the reporter for the running page — usable
// whether or not one was installed. Absence is ordinary: incremental sync
// installs no reporter, and neither do a connector's own tests.
func BackfillProgressFrom(ctx context.Context) BackfillReporter {
	p, _ := ctx.Value(backfillProgressKey{}).(BackfillProgress)
	return BackfillReporter{to: p}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The ingress seam — what a unit hands the core when it pulls a record out of
// a provider — is part of the published extension surface.
//
//margince:extension-surface

package extension

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// RecordKind is which kind of record a unit lands. It is the vocabulary a unit
// DECLARES in IngressSource.Lands and the vocabulary the core gates a call
// against, so a unit that declared one kind cannot land another.
type RecordKind string

// KindActivity is a captured interaction bound for the shared timeline.
//
// It is the only kind today, and the absence of a lead kind is deliberate
// rather than pending: the sink's lead arm returns before the row is written
// when a captured lead collides with an existing one, so the natural key never
// enters the table and the collision restages on every poll. Publishing a kind
// whose idempotency does not hold would be offering a unit a guarantee this
// side cannot keep.
const KindActivity RecordKind = "activity"

// ActivityKindMessage is the timeline kind a message on a messaging channel
// lands as, published here because a unit cannot reach the core's own
// vocabulary and would otherwise spell the string blind.
//
// It is the ONE kind whose transport is separable from it (ADR-0107/A158): a
// message names what carried it on ActivityFields.ChannelProvider, and every
// other kind names nothing there. A fitness test outside this package holds
// this constant equal to the core's own, so the two cannot drift into a unit
// filing a kind the core no longer calls a message.
const ActivityKindMessage = "message"

// IngressSource is one provider a unit brings records in from: the unit's own
// stable key for it, and which kinds it lands.
//
// It is inert data, exactly like SecretsRequest — declaring a source reaches no
// provider and lands nothing. What it buys is that `source_system` is derived
// from a DECLARED value rather than from whatever a call passes, so a typo is a
// refusal instead of a silently-minted second provenance namespace, and an
// operator reading manifest.generated.json can see which units reach core
// capture before one of them runs.
type IngressSource struct {
	// System is the unit's own key for the provider, lower snake/kebab, and
	// stable: it becomes half of every landed record's natural key, so changing
	// it re-lands the unit's whole history under a new identity.
	System string

	// Lands are the record kinds this source produces. Empty is invalid rather
	// than meaning "all": a declaration an operator reads must say what it does.
	//
	// One kind is landable today (KindActivity) and a Record carries exactly
	// one payload, so this list currently states what the call could not
	// contradict anyway. It is the growth point: the second kind arrives as a
	// second Record field, and the gate that a unit may land only what it
	// declared arrives with it, in the core.
	Lands []RecordKind

	// Merges are the identity keys this source's provider VOUCHES for, and the
	// core admits no other from it. A unit fills every field its provider gives
	// it; which of those the resolution ladder may treat as identity is read
	// from here rather than from whatever a record happens to carry.
	//
	// Empty — the default — means this source contributes no identity key. A
	// source that says nothing gets no power it did not ask for.
	Merges []MergeKey
}

// systemGrammar bounds the declared system key to what can sit inside a
// provenance string and a natural key without quoting: lower-case alphanumeric
// segments joined by single hyphens, the same shape a unit name takes.
var systemGrammar = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// maxSystemLength bounds the declared key's share of the stored provenance
// string. `ext:<unit>:<system>` is written into activity.source, which every
// timeline read carries, and a unit name is already capped at 32.
const maxSystemLength = 32

// Validate enforces what a source must state to be usable. It is the published
// check the manifest generator and the boot preflight both run, so generation
// time and boot time cannot disagree about which declarations are legal.
func (s IngressSource) Validate() error {
	switch {
	case strings.TrimSpace(s.System) == "":
		return errors.New("extension: a declared ingress source has an empty system key")
	case len(s.System) > maxSystemLength:
		return fmt.Errorf("extension: ingress system %q is %d characters — it is stamped into every landed record's provenance, so it is capped at %d",
			s.System, len(s.System), maxSystemLength)
	case !systemGrammar.MatchString(s.System):
		return fmt.Errorf("extension: ingress system %q is not a system key (lower-case [a-z0-9] segments joined by single hyphens)", s.System)
	case len(s.Lands) == 0:
		return fmt.Errorf("extension: ingress source %q declares no record kinds — a declaration an operator reads must say what it lands", s.System)
	}
	for _, kind := range s.Lands {
		if kind != KindActivity {
			return fmt.Errorf("extension: ingress source %q declares record kind %q — the only kind an ingress may land is %q",
				s.System, string(kind), string(KindActivity))
		}
	}
	for _, key := range s.Merges {
		if key != MergeKeyEmail {
			return fmt.Errorf("extension: ingress source %q declares merge key %q — the only identity key a source may vouch for is %q",
				s.System, string(key), string(MergeKeyEmail))
		}
	}
	return nil
}

// Message direction, relative to the member whose connection produced the
// record. Restated here rather than taken from the core's connector port,
// because the published surface may not export a type from beneath it.
const (
	DirectionInbound  = "inbound"
	DirectionOutbound = "outbound"
)

// Counterparty is the party on the other side of one captured message.
//
// It is identified by EMAIL or by a channel identity, and which one NAMES the
// human is the core's decision, not a unit's: a channel identity outranks an
// address, because it is the key a reply is routed on and the one a contact is
// bound by. A record may carry both — the address then corroborates rather than
// names, and the core admits it only from a source that declared
// MergeKeyEmail.
//
// So a unit supplies every field its provider gives it and decides nothing
// about identity. Dropping an address it holds, to fit a shape, would discard
// evidence the resolution ladder is entitled to.
//
// DisplayName is untrusted text — whatever the provider says the human calls
// themselves — and is bounded and stripped by the core before it is stored.
type Counterparty struct {
	Email       string
	DisplayName string
	// Domain is the lower-cased mail domain. It is what the core's own
	// suppression gates key on, and a unit that leaves it empty is not opting
	// out of them — it is failing to answer, which those gates read as "keep".
	Domain string
	// Direction is relative to the connected member: a message they received is
	// inbound. A record with no honest direction leaves it empty.
	Direction string
	// ChannelIdentity is the channel twin of Email: the account this party holds
	// at a messaging provider, for a record that has no address to carry.
	//
	// It is what makes a captured message REPLIABLE. The core binds it to the
	// contact it resolves, and the reply path resolves its recipient from that
	// binding — so a channel record that leaves this empty lands a message
	// nobody can answer, which looks exactly like a message with no reply box.
	//
	// Zero for every record that identifies its counterparty by address.
	ChannelIdentity ChannelIdentity
}

// ActivityFields is a captured interaction bound for the timeline. It mirrors
// the core's own shape; a fitness test outside this package compares the two so
// a field added there cannot go unnoticed here.
type ActivityFields struct {
	// Kind is the timeline kind — the core admits a closed set, and a value
	// outside it is refused rather than coerced.
	Kind string
	// ChannelProvider is the transport that carried this record, and it is
	// meaningful for exactly one kind: ActivityKindMessage.
	//
	// The pairing rule — a message names a transport, nothing else does, and a
	// unit may name only a transport it DECLARED — is enforced by the core at
	// the write door rather than here. This package does not know the core's
	// kind vocabulary and cannot see which channels the calling unit declared,
	// so a rule spelled here would be a second version of one that can disagree
	// with the first.
	ChannelProvider string
	// Subject and Body are the message as a human reads it. Both are bounded
	// (see the Max* constants): a provider is a remote party, and an unbounded
	// field is that party choosing how much this installation stores.
	Subject string
	Body    string
	// OccurredAt is when the message happened at the PROVIDER, not when the
	// poll saw it — a timeline ordered by discovery is a timeline of this
	// system's own scheduling.
	OccurredAt time.Time
	Direction  string
}

// The bounds every ingested record is held to, applied before any transaction
// opens. They are caps on what a REMOTE party may cause this installation to
// store per message, which is why they sit on the published surface rather than
// only in the adapter: a unit can check them itself and skip a doomed call.
const (
	// MaxRawBytes caps the provider's original record. Raw capture is evidence
	// and is written on every landed record, so this is the largest single
	// contribution one message makes to storage.
	MaxRawBytes = 256 * 1024
	// MaxKeyLength caps each half of the natural key. The key is indexed and
	// read back on every replay.
	MaxKeyLength = 256
	// MaxSubjectRunes caps a subject line — generous next to any subject a
	// human writes, small next to anything that would hurt a list rendering it.
	MaxSubjectRunes = 500
	// MaxBodyRunes caps a message body.
	MaxBodyRunes = 32768
	// MaxAddresses caps how many parties one record may enumerate.
	MaxAddresses = 64
	// MaxAddressLength is RFC 5321's ceiling: 64 octets of local part, an @,
	// and 255 of domain.
	MaxAddressLength = 320
	// MaxDisplayNameRunes caps untrusted display text.
	MaxDisplayNameRunes = 200
	// MaxThreadKeyLength caps the conversation key, which is stored on the
	// activity and joined against.
	MaxThreadKeyLength = 512
	// MaxChannelUserIDLength caps a provider's own account id. It is indexed
	// (the identity binding is unique per provider and account) and it is
	// remote-party text like every other bound here.
	MaxChannelUserIDLength = 256
	// MaxParticipants caps a record's roster, and it is a SHAPE guard rather
	// than a performance one: a message naming more contacts than this is a
	// broadcast list, and every name on it is evidence of a list membership
	// rather than of a conversation.
	//
	// Past it Record.Validate refuses the whole record, so a unit is TOLD
	// rather than left believing a sixty-contact group landed as one. A fitness
	// test outside this package holds the number equal to the core's own bound,
	// so a unit that checks itself against this and a core that applies its own
	// cannot answer differently about the same group.
	MaxParticipants = 50
)

// Record is one provider record on its way into the CRM.
//
// WHAT IT DELIBERATELY DOES NOT CARRY, because each absence is a decision:
//
//   - No Source and no CapturedBy. Both are stamped by the core from the
//     invoking unit's own identity, so a unit cannot attribute its records to
//     another unit or to a core connector. They are absent rather than
//     validated: a field a caller may not set, that the caller can still set,
//     is a validation rule somebody eventually relaxes.
//   - No Links. A record naming the core rows it attaches to would make the
//     core's link-visibility probe a per-row existence oracle over the scope
//     the ingest runs under. What a message is about is decided by the core's
//     own counterparty resolution, from the addresses below.
type Record struct {
	// System names which of the unit's declared ingress sources this came from.
	System string

	// Key is the provider's own identifier for this record — the half of the
	// natural key the unit supplies.
	//
	// It MUST be something the provider reports identically on a re-read. That
	// is the one obligation here a unit can silently get wrong: a key derived
	// from a timestamp, a page position, or the unit's own row id produces a
	// second copy of the record on every poll, and nothing fails.
	Key string

	// KeyNamesAContact reports that Key embeds the provider's identifier for a
	// HUMAN — a chat id that is the customer's own account id, say — rather
	// than naming a message, a notification or an event.
	//
	// It decides whether the pipeline trace stores the key or a hash of it, and
	// the unit answers because the core cannot: a key is opaque to it. Leave it
	// false for the ordinary case, which is what a provider id almost always
	// is; a message id in the trace is what makes a support question
	// answerable, and ADR-0082 §1 permits it for exactly that reason.
	KeyNamesAContact bool

	// Activity is the record itself. One field rather than a kind-tagged union
	// because one kind is landable today (see KindActivity); a second kind
	// arrives as a second field plus a rule that exactly one is set, which
	// leaves every unit written against this compiling.
	Activity ActivityFields

	// ThreadKey is the conversation this belongs to, and it MUST be namespaced
	// by the provider — a bare provider id shares one column with every other
	// source, where two of them can collide and join a stranger's conversation
	// onto this one.
	ThreadKey string

	// Counterparty is the other end of the exchange.
	Counterparty Counterparty

	// Addresses is EVERY party this record names, including the connected
	// member's own. At least one, and none of them blank.
	//
	// REQUIRED, and the reason is easy to get wrong: the core decides whether a
	// message is purely internal — colleagues talking, which is not evidence of
	// a customer relationship — by asking whether every party is on the
	// installation's own domains, and that question has no answer over an empty
	// set. It resolves to "not internal", so an empty set does not opt OUT of
	// the gate, it silently disables it and every record lands, colleagues
	// included. A blank ELEMENT is the same hole one party at a time, because
	// the gate skips what it cannot read.
	//
	// Which is why the grammar refuses both rather than describing them: a
	// connector that cannot enumerate the parties of a message cannot land it
	// honestly, and a refusal it can see beats a gate it cannot.
	Addresses []string

	// Participants is everyone else the record names — the roster of a group
	// conversation, the contacts neither end of the exchange.
	//
	// It answers "who was in the room", and that is ALL it answers. A party here
	// is recorded on the timeline and resolved to a contact the installation
	// already holds; none of it grants anybody a read. A unit cannot make a
	// colleague a reader by naming them, and it should not try: a roster is a
	// remote system's text, and the core keeps the mail rule over it — the same
	// rule that refuses to bind a colleague from a Cc line a sender typed.
	//
	// Optional and bounded by MaxParticipants. Past the cap the RECORD is
	// refused, not silently trimmed: a message naming a hundred contacts is a
	// broadcast list, half of one reads like a small conversation, and a unit
	// that reads the refusal can decide what its provider actually sent.
	Participants []Participant

	// Raw is the provider's record as received, kept as evidence.
	Raw []byte
}

// Participant is one further party to a record: someone in the conversation who
// is neither the connected member nor the counterparty.
//
// A unit fills every field its provider gives it. Account is what a chat has and
// mail does not — the provider's own id for the human — and Email is what mail
// has and a chat usually does not. Naming only one of them is the ordinary case,
// naming neither is a party with no identity and is dropped.
type Participant struct {
	// Account is the provider's account id for this party, on the transport the
	// record itself names (ActivityFields.ChannelProvider). It carries no
	// provider of its own: a party to THIS message is on the channel that
	// carried it, and a second spelling of that is one that can disagree.
	Account string
	// Email is this party's address where the provider gives one.
	Email string
	// Name is what the provider says the human calls themselves. Untrusted
	// text, bounded and stripped by the core before it is stored, and never
	// used to identify anybody.
	Name string
	// Role is where this party stood — one of the ParticipantRole constants. A
	// group chat's roster is ParticipantRoleAttendee: present, without being
	// either end of the exchange.
	Role string
}

// The roles a further party may hold, published because the set is closed at
// the core's own database and a unit spelling one blind would land a refusal
// rather than a record.
//
// They name a POSITION, not a direction. The two ends of an exchange are
// assigned from Counterparty and the member's own connection, never from here.
const (
	ParticipantRoleTo        = "to"
	ParticipantRoleCC        = "cc"
	ParticipantRoleAttendee  = "attendee"
	ParticipantRoleOrganizer = "organizer"
)

// ParticipantRoles is the published set as a list, so the validator, a unit's
// own checks and the fitness test that holds these equal to the core's all read
// ONE enumeration. A constant added above and left out here is a role the
// validator would refuse for being spelled correctly.
//
// `bcc` is deliberately absent though the core admits it: a bcc line exists
// only on the SENDER's own copy of a message, and a unit hands over a record it
// received. Publishing it would offer a position no unit can honestly report.
var ParticipantRoles = []string{
	ParticipantRoleTo,
	ParticipantRoleCC,
	ParticipantRoleAttendee,
	ParticipantRoleOrganizer,
}

// Disposition is what became of an ingested record. It exists because a row is
// not the only success: the core drops a wholly-internal message on purpose,
// committing a breadcrumb that says so, and a seam that reported that as a
// failure would have its caller retry a deliberate drop forever.
type Disposition string

const (
	// DispositionAccepted is a record that is now in the CRM, and the Ref names
	// it.
	//
	// It deliberately does NOT distinguish a row this call created from one a
	// previous call already had. The pipeline does not report that difference —
	// an idempotent upsert answers with the same reference either way — so a
	// published "replayed" would be a promise this side cannot keep, and a unit
	// reading it would be reading a value the core never produces. A unit that
	// wants new-versus-seen counts already knows: it is the one holding the
	// cursor.
	DispositionAccepted Disposition = "accepted"

	// DispositionSkipped is a record the core deliberately did not keep — a
	// message every party to which is on the installation's own domains, which
	// is colleagues talking rather than evidence of a customer relationship.
	//
	// It is a SUCCESS carrying a zero Ref. The core committed a breadcrumb
	// recording the drop, and its own contract is that a skip advances a
	// connector's watermark, so a unit treats this exactly as it treats
	// Accepted: move the cursor past it.
	DispositionSkipped Disposition = "skipped"

	// DispositionUnrepresentable is a record the core's grammar refuses: the
	// unit built something this contract cannot express.
	//
	// It was an ErrInvalid, and that put the one distinction a unit most needs
	// into error matching. A unit polling a provider cannot stop on a single
	// malformed message — that parks the whole connection over one record — so
	// it moves its cursor past it either way. What it could not do was tell
	// "the core skipped this deliberately" from "I built something the core
	// cannot use", because the first arrived as a Disposition and the second as
	// an error class, and a provider format change that made EVERY record
	// unrepresentable then presented exactly like a healthy quiet feed.
	//
	// It is a SUCCESS carrying a zero Ref, on the same terms as Skipped: the
	// cursor advances, because waiting does not make a malformed record
	// representable. And it is the one disposition a unit must COUNT. A run
	// answering it for everything it pulled is a broken unit or a changed
	// provider, and nothing else in the system is in a position to say so —
	// the core writes no breadcrumb for it yet (the second half of the seam,
	// which needs a table and its own decision about what such a row may hold).
	//
	// A unit switching on Disposition without a default gains an unhandled
	// case here. Go does not enforce exhaustiveness, so nothing stops
	// compiling; a unit that wants the count treats an unknown disposition as
	// this one, which is the safe direction — an unrecognised outcome is not an
	// acceptance.
	DispositionUnrepresentable Disposition = "unrepresentable"
)

// Ref names a record the core holds.
type Ref struct {
	Type string
	ID   string
}

// Result is what one ingest did.
//
// EVERY DISPOSITION ADVANCES A CURSOR, and that is the point of returning one
// instead of an error: a unit's watermark moves past every record the core has
// finished deciding about, whether or not a row came of it — a deliberate skip
// and a record the grammar refuses are both finished, and re-offering either on
// the next poll would repeat it forever.
type Result struct {
	Ref         Ref
	Disposition Disposition
	// Reason says WHY, for the dispositions that are a refusal. It carries the
	// core's complaint about an unrepresentable record so a unit can log the
	// same sentence it used to read off the error — and it is empty for an
	// acceptance and for a deliberate skip, where there is nothing the unit
	// could act on.
	//
	// Prose for a log, never a value to branch on: the Disposition is what a
	// unit decides from, and a unit parsing this string would be reading core
	// text that is free to change.
	Reason string
}

// The refusals an ingest can answer. Sentinels rather than typed errors,
// for the reason the core port's are: a unit's only reasonable response to each
// is a decision, and the core's own error text must not become a string some
// unit parses.
var (
	// ErrIngressNotDeclared is a unit that declared no ingress source, or named
	// one it did not declare. The manifest is the contract: a unit reaches core
	// capture only where an operator can see that it does.
	ErrIngressNotDeclared = errors.New("extension: this unit declared no such ingress source")

	// ErrAttendedIngest is an ingest from an invocation that has a caller.
	//
	// It is the exact mirror of the core port refusing a job tick. Ingress runs
	// on the authority of the member whose credential produced the record, and
	// an invocation that ALSO has a caller has two authorities in play — which
	// is the shape where a low-privileged caller has a unit act as somebody
	// else and reads the answer. A unit wanting an on-demand sync enqueues its
	// job rather than ingesting inline.
	ErrAttendedIngest = errors.New("extension: a record is ingested by an unattended run, never on a caller's invocation")

	// ErrNestedIngest is an ingest from inside the unit's own transaction. The
	// core's capture pipeline opens its own, so this would take a second
	// connection while holding one — which on a small pool does not fail, it
	// hangs.
	ErrNestedIngest = errors.New("extension: a record cannot be ingested from inside a transaction the unit is holding")
)

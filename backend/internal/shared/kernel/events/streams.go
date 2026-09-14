// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package events

// What the streams are CALLED, and which one an event rides. Apart from the
// catalog beside it because they answer different questions: this file is the
// key space the bus is enumerated, subscribed and purged over, and it is
// complete without knowing a single event type. catalog.go is the closed set
// of types, and reads the names from here.

import (
	"regexp"
	"sort"
)

// StreamPrefix namespaces every CRM stream on the shared gw:events bus
// (03-architecture §3.4: the same bus Dispact rides).
const StreamPrefix = "gw:events:crm:"

const (
	contactStreamEntity   = "contact"
	companyStreamEntity   = "company"
	dealStreamEntity      = "deal"
	leadStreamEntity      = "lead"
	activityStreamEntity  = "activity"
	approvalStreamEntity  = "approval"
	captureStreamEntity   = "capture"
	coldstartStreamEntity = "coldstart"
	auditStreamEntity     = "audit"
	identityStreamEntity  = "identity"
	voiceStreamEntity     = "voice"
)

// streamOverlay is the §5.10 overlay-mirror stream's entity segment — named
// once because the catalog below repeats it across every mirror.* entry.
const streamOverlay = "overlay"

// extensionStreamEntity is the one stream every EXTENSION-authored event rides,
// whichever unit published it.
//
// One stream for the whole tier rather than one per unit, because the stream SET
// is static everywhere it is read — the ops surface enumerates it, the purge
// unlinks it, a consumer group declares its streams at construction — while the
// composed unit set is a property of the build. Per-unit streams would make
// every one of those depend on which units an installation happens to ship.
const extensionStreamEntity = "extension"

// aiTaskStreamEntity is the stream every ai_task.state_changed rides.
//
// It is deliberately NOT in streamEntities, which is what coreStreams() — and
// therefore every all-stream group — expands to. An AI task's state change is
// an internal projection feed: the automation engine has no trigger for it, the
// webhook deliverer has no public type to name it by, and the audit stream's
// agent-actor slice already sees the work itself through the events that
// mutated records. Enumerated by Streams() all the same, because a stream the
// purge does not unlink is one that outlives a data reset.
const aiTaskStreamEntity = "aitask"

// briefStreamEntity is the stream every brief.* product-telemetry event rides.
//
// Out of streamEntities for the same three reasons aiTaskStreamEntity is, and
// they hold one at a time: the automation engine has no trigger for "a rep
// opened their Brief", the webhook deliverer has no public type to name it by,
// and the audit stream already carries what the rep DID — brief_item marks
// write audit rows — so an all-stream consumer would gain nothing but volume.
// What this stream answers is the one thing those cannot: whether the page was
// opened at all. Enumerated by Streams() all the same, so the purge unlinks it.
const briefStreamEntity = "brief"

// ExtensionEventVersion is the payload schema version every extension event
// carries, and it is 1 forever.
//
// A unit changes the shape of what it publishes by naming a NEW VERB, never by
// bumping this. The alternative is version negotiation across a boundary with no
// shared schema registry: the core cannot validate a unit's payload against
// anything, so a number here would be a promise neither side could keep, and a
// consumer trusting it would be trusting the publisher's own word about a shape
// nobody checked.
const ExtensionEventVersion = 1

// extensionTypeGrammar is the extension event type law, `ext_<namespace>.<verb>`
// — the SQL namespace a unit owns (`ext_` plus its name with hyphens turned to
// underscores), then a lower snake_case verb.
//
// The namespace half mirrors what a unit NAME can actually derive: the name
// grammar admits `[a-z0-9]+` segments joined by SINGLE hyphens, so the
// namespace is those segments joined by single underscores and nothing else.
// Spelling it that way rather than as a loose `[a-z0-9_]+` is what makes
// `ext__notes.x` and `ext_notes_.x` unroutable — neither is a namespace any
// unit could own, so a subscription declaring one is a typo the boot can catch
// instead of a listener that quietly never fires.
//
// The namespace half keeps one unit's events out of another's name and out of
// the core families entirely, and it is the same token that opens the unit's
// tables and its database role, so a reader who knows one knows the others.
// Nothing here is a grant: the publisher's namespace is derived from the
// INVOCATION at the port, where a unit never spells it, and this grammar only
// says what such a type looks like so the bus can route one.
var extensionTypeGrammar = regexp.MustCompile(`^ext_[a-z0-9]+(_[a-z0-9]+)*\.[a-z][a-z0-9_]*$`)

// IsExtensionType reports whether an event type is extension-authored. It is a
// question about SHAPE, not a registry lookup: the catalog below is the core's
// closed set, while the extension set is whatever the composed units publish —
// which no file in this repository can enumerate.
func IsExtensionType(eventType string) bool {
	return extensionTypeGrammar.MatchString(eventType)
}

// streamEntities are the V1 family streams from events.md, plus the §5.6a
// identity/access-revocation stream, the voice owner-private lifecycle
// stream, and the §5.10 overlay-mirror stream (overlay-mode-only).
// Workspace is a field inside the envelope, never a stream —
// per-tenant streams would explode key count at multi-tenant scale.
var streamEntities = []string{
	contactStreamEntity, companyStreamEntity, dealStreamEntity, leadStreamEntity, activityStreamEntity,
	approvalStreamEntity, captureStreamEntity, coldstartStreamEntity, auditStreamEntity, identityStreamEntity, voiceStreamEntity,
	streamOverlay,
}

// Streams returns the full stream key set, sorted, for the ops surface to
// enumerate — the core families AND the extension tier's one stream. A stream
// left out here is one the data reset does not unlink and no operator can see,
// which is a worse outcome than the entries it would leave behind.
func Streams() []string {
	out := make([]string, 0, len(streamEntities)+3)
	for _, e := range streamEntities {
		out = append(out, StreamPrefix+e)
	}
	out = append(out, StreamPrefix+extensionStreamEntity, StreamPrefix+aiTaskStreamEntity,
		StreamPrefix+briefStreamEntity)
	sort.Strings(out)
	return out
}

// coreStreams is what a CORE consumer group means when it subscribes to
// "everything": the family streams, and deliberately NOT the extension one.
//
// A core consumer has nothing to do with a unit's event and no way to act on
// one. The automation engine would load every live instance and match no
// trigger; the webhook deliverer would query subscriptions for a type the
// public catalog cannot name. Both are pure cost on every extension event, both
// are invisible, and both grow with the tier. A unit's event is delivered to
// the units that ASKED for it, through their own groups.
func coreStreams() []string {
	out := make([]string, len(streamEntities))
	for i, e := range streamEntities {
		out[i] = StreamPrefix + e
	}
	sort.Strings(out)
	return out
}

// ExtensionStream is the stream key every extension-authored event rides. The
// composition builds a unit's consumer group over it by name, and the routing
// test pins that no core group carries it.
func ExtensionStream() string {
	return StreamPrefix + extensionStreamEntity
}

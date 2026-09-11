// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package events

import (
	"fmt"
	"sort"
	"strings"
)

// catalog is the enumerable V1 event catalog (events.md §5.1–§5.10, plus
// the §5.11 signal lifecycle): each type's home stream entity and current
// payload schema version. §5.10 (overlay mirror) is overlay-mode-only —
// these types are only ever emitted for an installation with overlay_mode.sor_mode =
// 'overlay' — and the remaining §5.11 type (forecast.period_closed) rides
// E09 — deferred with its work package.
//
// Types whose entity segment is not itself a stream ride their family's
// stream (events.md §1 routing rule): consent.*/retention.* are
// person-lifecycle events, offer.*/pipeline.*/stage.* belong to the
// deal family — each declares its stream home here, and no catalog type
// may imply a stream §4.1 does not define.
var catalog = map[string]struct {
	stream  string
	version int
}{
	"person.created":  {personStreamEntity, 1},
	"person.updated":  {personStreamEntity, 1},
	"person.archived": {personStreamEntity, 1},
	"person.merged":   {personStreamEntity, 1},
	"person.restored": {personStreamEntity, 1},
	"consent.changed": {personStreamEntity, 1},
	// Somebody recorded that we may not write to a subject. Its own type rather
	// than a consent.changed, because a suppression is not the absence of
	// consent: it outranks a grant and a later re-grant does not erase it, so a
	// consumer folding the two would resume mail the subject asked us to stop.
	"consent.suppressed": {personStreamEntity, 1},
	// A stop taken back by somebody who outranked the level that set it. Its own
	// type because a consumer that saw only the suppression would keep treating
	// the subject as stopped forever, which is the state this event exists to
	// end.
	"consent.suppression_lifted": {personStreamEntity, 1},
	// What a contact promised, asked or decided, and a human's correction of
	// it. Both ride the PERSON stream: a subscriber reacting to what somebody
	// said wants the person, and the claim id rides the payload for the reader
	// that needs the row. A correction is published where a moment dismissal is
	// not, because a correction is shared truth and a dismissal is one screen.
	"conversation_claim.captured": {personStreamEntity, 1},
	"conversation_claim.changed":  {personStreamEntity, 1},
	// A member recording or correcting their own LinkedIn authorization. It
	// rides the person stream because the thing it governs is whose network
	// gets read, and consent to read a professional network is the same class
	// of fact as consent.changed beside it.
	// The sender's own sign-off. It rides the person stream because what it
	// governs is how a member is represented on every message they send, which
	// is a fact about that person rather than about any one mail.
	"email_signature.changed": {personStreamEntity, 1},
	// The language a member reads their interface in, and the name colleagues
	// see them by. Both ride the person stream for the same reason the sign-off
	// does: they are facts about that person, not about the installation, which
	// names its own language in a setting and publishes nothing per reader.
	"user_locale.changed":       {personStreamEntity, 1},
	"user_display_name.changed": {personStreamEntity, 1},
	// What a member wants DELIVERED rides the identity stream rather than the
	// person one its neighbour above uses. A display language is something a
	// subscriber rendering for this person needs; what lands in their inbox is
	// nobody else's business, and the stream is the first place that is said.
	"user_delivery.changed":    {identityStreamEntity, 1},
	"linkedin_account.changed": {personStreamEntity, 1},
	// One import act, not one row: an export is thousands of rows and a
	// per-row event would bury every other event in the stream, while the
	// auditable fact is that a member imported their network at all.
	"linkedin_network.imported": {personStreamEntity, 1},
	// One decision on one connection. It rides the person stream because the
	// decision is ABOUT a contact — and it names neither the contact nor the
	// connection, because a ghost's identity must not travel through the bus.
	"linkedin_match.decided": {personStreamEntity, 1},
	"retention.applied":      {personStreamEntity, 1},
	// A statutory obligation withheld, released or pinned one activity
	// (A165/ADR-0114). It rides the person stream beside retention.applied:
	// it is the erasure's other outcome, published from the same transaction,
	// and a subscriber tracking one has to see the other. Its own type rather
	// than a fourth retention.applied action, because `restrict` obliges the
	// subscriber to drop a record that still exists — an obligation no
	// existing action carries, so it must not reach a subscriber that never
	// opted into it.
	"retention.restricted": {personStreamEntity, 1},

	"company.created":  {companyStreamEntity, 1},
	"company.updated":  {companyStreamEntity, 1},
	"company.archived": {companyStreamEntity, 1},
	"company.merged":   {companyStreamEntity, 1},

	"deal.created":       {dealStreamEntity, 1},
	"pipeline.created":   {dealStreamEntity, 1},
	"pipeline.updated":   {dealStreamEntity, 1},
	"pipeline.archived":  {dealStreamEntity, 1},
	"stage.created":      {dealStreamEntity, 1},
	"stage.updated":      {dealStreamEntity, 1},
	"stage.archived":     {dealStreamEntity, 1},
	"deal.updated":       {dealStreamEntity, 1},
	"deal.stage_changed": {dealStreamEntity, 1},
	"deal.owner_changed": {dealStreamEntity, 1},
	"deal.archived":      {dealStreamEntity, 1},
	"deal.restored":      {dealStreamEntity, 1},
	// The project rides the deal family stream: it is the body of work the
	// deals hang off, and a consumer interested in one is interested in both.
	"project.created":       {dealStreamEntity, 1},
	"project.updated":       {dealStreamEntity, 1},
	"project.phase_changed": {dealStreamEntity, 1},
	"project.archived":      {dealStreamEntity, 1},
	// A commission entry rides the deal family stream for the same reason a
	// contract does: it is what a WON deal produced, and a consumer following
	// the commercial arc of one deal wants the money that came out of it on
	// the same stream as the win that caused it.
	"commission.accrued": {dealStreamEntity, 1},
	"commission.decided": {dealStreamEntity, 1},
	// A contract rides the deal family stream too: it is what a won deal
	// points at, and a consumer following the commercial arc wants both.
	"contract.created":        {dealStreamEntity, 1},
	"contract.updated":        {dealStreamEntity, 1},
	"contract.status_changed": {dealStreamEntity, 1},
	"contract.archived":       {dealStreamEntity, 1},
	"offer.created":           {dealStreamEntity, 1},
	"offer.sent":              {dealStreamEntity, 1},
	"offer.accepted":          {dealStreamEntity, 1},
	"offer.rejected":          {dealStreamEntity, 1},
	"offer.superseded":        {dealStreamEntity, 1},
	// A Deal Room rides the deal family stream for the same reason a contract
	// does: it is the buyer-facing face of one deal, and a consumer following
	// that deal's arc wants what the buyer was shown alongside what changed.
	"deal_room.opened":   {dealStreamEntity, 1},
	"deal_room.updated":  {dealStreamEntity, 1},
	"deal_room.paused":   {dealStreamEntity, 1},
	"deal_room.resumed":  {dealStreamEntity, 1},
	"deal_room.closed":   {dealStreamEntity, 1},
	"deal_room.archived": {dealStreamEntity, 1},
	// Access changes ride the same stream: who may read a deal's material is part
	// of that deal's arc, and a consumer following it wants both.
	"deal_room.participant_invited":             {dealStreamEntity, 1},
	"deal_room.participant_revoked":             {dealStreamEntity, 1},
	"deal_room.participant_credential_reissued": {dealStreamEntity, 1},
	// Editorial content (documents, wording) reaches a buyer through
	// deal_room.published and is not announced separately. The conversation is live on both sides and never waits for a publish, so
	// each act announces itself: a comment (on a document or the room), the
	// seller resolving a thread, and a buyer's decision on a document version.
	"deal_room.comment_posted":    {dealStreamEntity, 1},
	"deal_room.thread_resolved":   {dealStreamEntity, 1},
	"deal_room.decision_recorded": {dealStreamEntity, 1},

	"lead.created":      {leadStreamEntity, 1},
	"lead.updated":      {leadStreamEntity, 1},
	"lead.promoted":     {leadStreamEntity, 1},
	"lead.demoted":      {leadStreamEntity, 1},
	"lead.merged":       {leadStreamEntity, 1},
	"lead.sla_breached": {leadStreamEntity, 1},
	"lead.disqualified": {leadStreamEntity, 1},
	// The two lead vocabularies, on the lead stream because their entries are
	// values every lead carries: a subscriber that groups by source or reports
	// on why leads were disqualified has to re-read the catalog when one
	// changes. events.md §5.3b — config changes are first-class facts, the
	// same reason pipeline.created is published.
	"lead_source.changed":            {leadStreamEntity, 1},
	"lead_disqualify_reason.changed": {leadStreamEntity, 1},

	"activity.captured": {activityStreamEntity, 1},
	"activity.updated":  {activityStreamEntity, 1},
	"activity.archived": {activityStreamEntity, 1},
	// Somebody decided what to do about a waiting message and the Worklist
	// stopped offering it. `disposition_recorded` rather than `disposition_set`
	// because the catalog's verbs are past tense, and a compound one puts the
	// object first — the shape `password_link_issued` already takes.
	//
	// Its own type rather than an activity.updated: the
	// message did not change, only what one person (or the workspace) decided
	// about it, and a consumer counting edits to correspondence must not read a
	// rep clearing their queue as the customer's mail being rewritten.
	"activity.disposition_recorded": {activityStreamEntity, 1},
	// A rep set a lapsed CONTACT aside so their own decay lane stops raising
	// them, or put them back. The entity is the person, which is what the
	// judgement is about — the relationship's silence is a fact about them
	// rather than about any one message.
	"relationship_nudge.decided": {personStreamEntity, 1},
	// §5.11: a thread-matched inbound is an activity-family fact, emitted
	// by capture alongside activity.captured (EVT-SEM-14 — idempotent per
	// reply; a duplicate inbound for the same reply does not re-emit).
	"engagement.reply": {activityStreamEntity, 1},

	// A returned send is a fact about the sent activity, so it rides the
	// activity stream the send's own events ride.
	"comms.delivery_bounced": {activityStreamEntity, 1},

	// A notice is addressed to one person, so its lifecycle rides the
	// identity family's stream: created is the delivery on this transport,
	// read is the recipient settling it.
	"notice.created": {identityStreamEntity, 1},
	"notice.read":    {identityStreamEntity, 1},

	// A weekly plan belongs to one rep, so its changes ride the same identity
	// stream a notice does. help_requested is its own type rather than another
	// updated: it is the one change somebody else is meant to act on, and an
	// automation notifying a lead subscribes to that and not to every tick of
	// a checkbox.
	"weekly_plan.updated":        {identityStreamEntity, 1},
	"weekly_plan.help_requested": {identityStreamEntity, 1},

	// A call rides the identity stream because its entity is the AUTHOR. A
	// forecast is about a pipeline, but a CALL is an assertion by a person and
	// is attributable to them — a consumer asking "who said this number" is
	// asking about a user, not about a deal.
	"forecast.created":            {identityStreamEntity, 1},
	"forecast.exception_resolved": {identityStreamEntity, 1},
	"forecast.assurance_created":  {identityStreamEntity, 1},
	"forecast.snapshot_created":   {identityStreamEntity, 1},
	// A share is attributable to whoever issued it, and the issuer's standing
	// is what keeps it serving, so both halves ride the identity stream.
	"forecast.share_issued":  {identityStreamEntity, 1},
	"forecast.share_revoked": {identityStreamEntity, 1},

	// An introduction request is about a CONTACT — who can open a door to
	// them, and what came of asking — so it rides the person stream a
	// consumer ranking that contact's open work already reads.
	"intro_request.created":   {personStreamEntity, 1},
	"intro_request.decided":   {personStreamEntity, 1},
	"intro_request.completed": {personStreamEntity, 1},
	"intro_request.replied":   {personStreamEntity, 1},
	"intro_request.closed":    {personStreamEntity, 1},

	"approval.requested": {approvalStreamEntity, 1},
	"approval.decided":   {approvalStreamEntity, 1},

	"capture.received":   {captureStreamEntity, 1},
	"capture.normalized": {captureStreamEntity, 1},
	"capture.failed":     {captureStreamEntity, 1},
	"capture.skipped":    {captureStreamEntity, 1},

	// §5.11: signal is not one of the nine stream entities — the
	// detection lifecycle rides the capture stream (events.md §5.11
	// stream-routing rule).
	"signal.detected": {captureStreamEntity, 1},
	"signal.resolved": {captureStreamEntity, 1},

	"coldstart.read_back_proposed": {coldstartStreamEntity, 1},
	"coldstart.accepted":           {coldstartStreamEntity, 1},
	"coldstart.rejected":           {coldstartStreamEntity, 1},

	"audit.appended": {auditStreamEntity, 1},

	// §5.6a: the access-revocation cascade (B-EP03.10) — user, role and
	// passport are identity-owned facts, so all three ride the identity
	// stream rather than gaining per-entity streams of their own.
	"user.invited":              {identityStreamEntity, 1},
	"user.activated":            {identityStreamEntity, 1},
	"user.password_link_issued": {identityStreamEntity, 1},
	"user.deactivated":          {identityStreamEntity, 1},
	"user.reactivated":          {identityStreamEntity, 1},
	"role.changed":              {identityStreamEntity, 1},
	"team.changed":              {identityStreamEntity, 1},
	"passport.revoked":          {identityStreamEntity, 1},
	"onboarding.state_changed":  {identityStreamEntity, 1},

	"voice.profile_created":        {voiceStreamEntity, 1},
	"voice.profile_updated":        {voiceStreamEntity, 1},
	"voice.profile_archived":       {voiceStreamEntity, 1},
	"voice.corpus_changed":         {voiceStreamEntity, 1},
	"voice.build_changed":          {voiceStreamEntity, 1},
	"voice.version_changed":        {voiceStreamEntity, 1},
	"voice.draft_outcome_recorded": {voiceStreamEntity, 1},

	// §5.10: the overlay mirror's own stream — emitted only in overlay
	// mode. mirror.write_rejected is reserved for the branch-2 write
	// path but registered now so the catalog is complete.
	"mirror.conflict":        {streamOverlay, 1},
	"mirror.budget_degraded": {streamOverlay, 1},
	"mirror.write_rejected":  {streamOverlay, 1},
	"mirror.deleted":         {streamOverlay, 1},

	// §4.3: the incumbent connection lifecycle — a genuine SoR mutation
	// (unlike mirror ingest), so it carries the full write shape and
	// rides the same overlay-mode-only stream as the mirror it gates.
	"incumbent.connected":    {streamOverlay, 1},
	"incumbent.disconnected": {streamOverlay, 1},

	// The AI-activity projection feed (ai_task_run). One type with the state
	// inside, like voice.build_changed: a new state must never need a new type.
	"ai_task.state_changed": {aiTaskStreamEntity, 1},

	// Product telemetry: the morning Brief was read. Internal only — nothing
	// subscribes to it, and api/internal-events.yaml says why that file exists.
	"brief.opened": {briefStreamEntity, 1},
}

// IsPipelineEvent reports whether an event type is an entity-less
// pipeline-event class member (see pipelineEventTypes): its envelope may
// carry an empty Entity ref where a normal event must name its subject.
func IsPipelineEvent(eventType string) bool {
	_, ok := pipelineEventTypes[eventType]
	return ok
}

// Types returns every catalog event type, sorted — the enumerable set
// codegen and the naming fitness test walk.
func Types() []string {
	out := make([]string, 0, len(catalog))
	for t := range catalog {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// StreamFor routes an event type to its stream key. An unknown type is a
// programming error the publisher must surface before the outbox write —
// an unroutable row would wedge the relay forever.
//
// The catalog is consulted FIRST and the extension grammar second, so a core
// type can never be routed by shape. That ordering is belt to the braces of
// TestNoCatalogTypeLooksLikeAnExtensionType, which holds the two vocabularies
// apart at their source.
func StreamFor(eventType string) (string, error) {
	if spec, ok := catalog[eventType]; ok {
		return StreamPrefix + spec.stream, nil
	}
	if IsExtensionType(eventType) {
		return ExtensionStream(), nil
	}
	return "", fmt.Errorf("events: %q is neither an events.md §5 catalog type nor an ext_<namespace>.<verb> extension type", eventType)
}

// VersionOf returns the current payload schema version of a catalog type
// (0 for an unknown type; Validate rejects those via StreamFor first), and
// ExtensionEventVersion for an extension type. Publishers stamp envelopes
// from here — never a literal — so a future v2 bump happens in exactly one
// place, and so the extension port has no version of its own to get wrong.
func VersionOf(eventType string) int {
	if spec, ok := catalog[eventType]; ok {
		return spec.version
	}
	if IsExtensionType(eventType) {
		return ExtensionEventVersion
	}
	return 0
}

// SplitType breaks a catalog type into its <entity>.<verb> segments
// (events.md §1). Multi-word verbs keep their underscores
// ("stage_changed", "read_back_proposed").
func SplitType(eventType string) (entity, verb string, err error) {
	entity, verb, ok := strings.Cut(eventType, ".")
	if !ok || entity == "" || verb == "" {
		return "", "", fmt.Errorf("events: %q is not <entity>.<verb>", eventType)
	}
	return entity, verb, nil
}

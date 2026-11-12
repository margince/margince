// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// fieldKind names the activity kind in the write shape's audit and outbox
// payloads (the one spelling of the payload key).
const (
	fieldKind    = "kind"
	fieldSubject = "subject"
)

// Website consumers need the accepted identity, never a copy of the submitted text.
func activityCapturedPayload(ctx context.Context, in LogActivityInput) (crmcontracts.PublicEventActivityCaptured, error) {
	p := crmcontracts.PublicEventActivityCaptured{Kind: in.Kind}
	if in.Kind == string(crmcontracts.ActivityKindNote) && in.SourceSystem != nil && *in.SourceSystem == provenance.WebFormSource && in.SourceID != nil {
		p.SourceSystem = in.SourceSystem
		p.WebsiteSubmissionKey = in.SourceID
		owner, err := workflow.AcceptedInquiryOwner(in.Raw)
		if err != nil {
			return p, err
		}
		if owner != nil {
			id := crmcontracts.Id(*owner)
			p.HandlingAutomationId = &id
		}
	}
	// An import's activities are history, so consumers that react to new work
	// read the mark and skip them.
	p.Imported, p.ImportRunId = storekit.ImportMark(ctx)
	// Carried only when there is one, so omitempty leaves it absent. The kind
	// alone no longer says what carried a message.
	if in.ChannelProvider != "" {
		p.ChannelProvider = &in.ChannelProvider
	}
	return p, nil
}

type LogActivityInput struct {
	DueIntent     *crmcontracts.IntentTime
	RemindIntent  *crmcontracts.IntentTime
	MeetingIntent *crmcontracts.IntentTime
	// Queued mail keeps its original entity when the sender changes company.
	correspondenceEntity ids.UUID
	SystemText           *systemtext.Message
	identityProver       AddressProver
	DealFiling           *crmcontracts.ActivityDealFiling
	dealFilingResolver   DealFilingResolver
	Recommendation       *crmcontracts.RecommendationReceipt
	FollowUpTask         *LogActivityInput
	recordedAt           *time.Time
	// Internal creation policy; public activity input cannot set an audience.
	audienceMembers []AudienceMember
	// audienceOf gives the new activity the audience, members and parties of
	// the activity it records. Nil leaves the audience it is born with.
	audienceOf *ids.ActivityID
	// invitedAssignee admits an invited colleague as the task's assignee. The
	// HTTP create handler sets it for an assignee the caller NAMED, and
	// AdmitInvitedAssignee for a promiser's own task. Every other automatic
	// writer leaves it false and hands work to active seats only.
	invitedAssignee bool
	Kind            string
	// LinksFromRequest says the caller typed Links, so a missing record is
	// refused naming `links`. A writer that derives its links leaves it false.
	LinksFromRequest bool
	// ChannelProvider names the messaging transport that carried this activity —
	// a channel_provider row — and is empty for anything that did not travel on
	// one. Separate from Kind because they answer separate questions: what sort
	// of interaction happened, versus how it travelled (ADR-0107/A158).
	//
	// Non-empty exactly when Kind is KindMessage, which the database enforces in
	// both directions; a mismatch is a 422 from the CHECK, not a silent write.
	ChannelProvider string
	Subject         *string
	Body            *string
	OccurredAt      *time.Time
	// OccurredPrecision is PrecisionDay when OccurredAt is only a date's anchor;
	// empty is an instant.
	OccurredPrecision string
	Direction         *string
	// MeetingStatus is meaningful only for kind meeting; the mapping refuses
	// it on any other kind. The lead status ladder reads booked/held as
	// engagement, so a hand-logged meeting moves a lead the same as a synced
	// one.
	MeetingStatus *string
	// CallOutcome is how a call ended; the mapping refuses it on another kind.
	CallOutcome *string
	DueAt       *time.Time
	// DueHasTime is true when DueAt names an agreed time of day rather than
	// the day the task is due by. It needs DueAt, and on a task with no
	// RemindAt it sets the reminder to DueAt (admitTaskTime).
	DueHasTime bool
	RemindAt   *time.Time
	AssigneeID *ids.UserID
	HostUserID *ids.UserID
	// AttendeeUserIDs are the colleagues who were in a meeting or call besides
	// whoever logged it; each is stamped as an our-side participant.
	AttendeeUserIDs []ids.UserID
	// SenderContactID is the linked contact who wrote an inbound activity. Nil
	// leaves the sender to stampLoggedCounterparties' rule for the link count.
	SenderContactID *ids.UUID
	// ClaimsHostSlot marks a meeting that HOLDS its host's hour, which is what
	// activity_meeting_no_overlap refuses a second of. Only BookMeeting sets
	// it: a booking door takes a slot on somebody's calendar, and taking one
	// twice is the fault the guard exists for. Logging a meeting records that
	// one happened and claims nothing — a rep writing up a morning of
	// back-to-back calls is describing their Tuesday, not double-booking it.
	//
	// There is no way to set it from the wire, and there must not be: a guard a
	// caller can opt out of is not one.
	ClaimsHostSlot bool
	SourceSystem   *string
	SourceID       *string
	// Author is who wrote it in the system it came from; zero when unknown.
	Author storekit.SourceAuthorInput
	// SourceActivityID is the activity this one was derived FROM — the meeting
	// whose transcript proposed a task. Nil on almost every activity.
	SourceActivityID  *ids.UUID
	RequestActivityID *ids.UUID
	// ThreadKey files this activity under a conversation. Empty stores NULL.
	// It is written at insert time or not at all: the (source_system,
	// source_id) upsert both capture and this path key on does nothing when
	// the row already exists, so neither leg can revise the other's value.
	ThreadKey string
	// InReplyTo is the email this one replies to: the reply takes that mail's
	// thread (threadOnReplyTx). Nil on every other activity.
	InReplyTo *ids.UUID
	// ownThread files a hand-logged email with no thread of its own under
	// ownThreadKey of its id. Only the create mapping sets it.
	ownThread bool
	// CounterpartyEmail is the address this message was with, normalized —
	// the column capture's correspondence-positive gate (ADR-0072 §1) reads.
	// CounterpartyOutboundAttested says the workspace itself sent to that
	// address; it is affirmative intent toward them, and it is what spares
	// their later mail from suppression. Both obey the same write-once rule
	// as ThreadKey, for the same reason.
	CounterpartyEmail            string
	CounterpartyOutboundAttested bool
	// Raw is the source system's own representation of this activity, kept
	// verbatim. It is CONTENT: the audience projection withholds it from a
	// reader who may not read the subject and body, and both destructive
	// paths — retention and noise redaction — null it with the rest of the text.
	Raw *map[string]any
	// DurationSeconds is how long a meeting or call lasted; the mapping
	// refuses it on any other kind.
	DurationSeconds *int
	// The address headers an importer stated, normalized and each address in
	// one role only. Capture derives these from the message it holds; an
	// importer holds only what its source system kept, so it states them.
	EmailFrom string
	EmailTo   []string
	EmailCc   []string
	// RFCMessageID is this message's own RFC 5322 identity, brackets stripped —
	// what a later capture of the same message resolves against.
	RFCMessageID string
	// The calendar identity, which is a PAIR: a recurring series shares one UID
	// across every occurrence, so the UID names the series and the instance
	// names the occurrence within it.
	ICalUID      string
	ICalInstance string
	Links        []ActivityLinkInput
	Source       string
	// CustomFields are the create wire's cf_ values, keyed by column name.
	// customColumns are the active fields they are checked and written against.
	CustomFields  map[string]any
	customColumns []fieldcatalog.Column
	customChecked bool
	// Origin says who caused this row to exist, and the recency clocks in the
	// schema read it: the two system origins are excluded from every
	// last_activity_at, because neither the system asking about a silent deal
	// nor the installation mailing somebody a notice it owes them is the other
	// side breaking their silence. Empty means OriginHuman.
	Origin string
}

// The origins an activity can have.
//
// Two of them are the system writing rather than a contact, and neither counts
// as the record being touched: OriginSystemRemediation marks work the product
// files about a record — a forecast-assurance review task — and OriginSystemNotice
// marks a message the installation owes somebody, such as the confirm-details
// link. A Go query that needs to exclude them calls auth.OriginIsEngagement
// rather than naming an origin itself; the last_activity_of_* functions carry
// the same clause in SQL.
//
// Held by: TestEveryRecencyReadingExcludesTheSystemOrigins (backend/gates/recencyorigins_test.go)
const (
	OriginHuman             = "human"
	OriginAgent             = "agent"
	OriginSystemRemediation = "system_remediation"
	OriginSystemNotice      = "system_notice"
)

// LogActivity writes the activity + links; the last_activity_at clocks
// (data-model §7) are maintained in the schema, not here. Idempotent on
// (source_system, source_id): replaying a capture returns the existing
// activity.
func (s *Store) LogActivity(ctx context.Context, in LogActivityInput) (crmcontracts.Activity, bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return crmcontracts.Activity{}, false, err
	}
	if err := s.withCustomColumns(ctx, &in); err != nil {
		return crmcontracts.Activity{}, false, err
	}
	var out crmcontracts.Activity
	created := true
	isolation := pgx.ReadCommitted
	if in.Recommendation != nil {
		isolation = pgx.Serializable
	}
	err := s.txRetryingLockCycles(ctx, isolation, func(tx pgx.Tx) error {
		var err error
		out, created, err = s.logActivityAndReadTranscript(ctx, tx, in)
		return err
	})
	if in.Recommendation != nil && storekit.IsLockCycle(err) {
		return out, created, fmt.Errorf("the suggested step changed while it was accepted; refresh it: %w", apperrors.ErrVersionSkew)
	}
	if err != nil {
		return out, created, err
	}
	return out, created, s.attachCustomValues(ctx, &out)
}

// LogActivityTx keeps the activity and its derived work in the caller’s transaction.
func (s *Store) LogActivityTx(ctx context.Context, tx pgx.Tx, in LogActivityInput) (crmcontracts.Activity, bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return crmcontracts.Activity{}, false, err
	}
	if err := s.withCustomColumns(ctx, &in); err != nil {
		return crmcontracts.Activity{}, false, err
	}
	// The columns this call already read, never a second read: the caller holds
	// its transaction, and the catalog would open another connection under it.
	out, created, err := s.logActivityAndReadTranscript(ctx, tx, in)
	if err == nil {
		err = withCustomValuesOf(ctx, tx, in.customColumns, &out)
	}
	return out, created, err
}

// All activity writers share transcript processing and recommendation admission.
func (s *Store) logActivityAndReadTranscript(
	ctx context.Context, tx pgx.Tx, in LogActivityInput,
) (crmcontracts.Activity, bool, error) {
	if err := CheckEntryHasContent(in.Kind, in.Subject, in.Body, in.CallOutcome); err != nil {
		return crmcontracts.Activity{}, false, err
	}
	if in.RequestActivityID != nil {
		if err := validateFollowUpShape(in); err != nil {
			return crmcontracts.Activity{}, false, err
		}
		return s.takeEmailRequest(ctx, tx, in)
	}
	if replay, found, err := s.replayedTaskRecommendation(ctx, tx, in); found || err != nil {
		return replay, false, err
	}
	if err := s.checkTaskRecommendation(ctx, tx, &in); err != nil {
		return crmcontracts.Activity{}, false, err
	}
	if err := refuseOwnThreadKeys(in); err != nil {
		return crmcontracts.Activity{}, false, err
	}
	if err := threadOnReplyTx(ctx, tx, &in); err != nil {
		return crmcontracts.Activity{}, false, err
	}
	at := s.now()
	in.recordedAt = &at
	in.dealFilingResolver = s.dealFilingResolver
	in.identityProver = s.identityProver
	out, created, err := logActivityInTx(ctx, tx, in)
	if err != nil {
		return out, created, err
	}
	if err := s.readTranscriptOnLanding(ctx, tx, out, created); err != nil {
		return out, created, err
	}
	if created && in.FollowUpTask != nil {
		followUp := *in.FollowUpTask
		followUp.SourceActivityID = new(ids.UUID(out.Id))
		if _, _, err := s.logActivityAndReadTranscript(ctx, tx, followUp); err != nil {
			return out, created, err
		}
	}
	return out, created, nil
}

// An importer records another system's history; the actor alone proves no attendance.
func meetingHost(ctx context.Context, in LogActivityInput) *ids.UserID {
	if in.HostUserID != nil || in.Kind != KindMeeting {
		return in.HostUserID
	}
	if in.Author.AuthorID != nil {
		host := ids.From[ids.UserKind](*in.Author.AuthorID)
		return &host
	}
	if in.Author.AuthorName != nil || isImportedActivity(in) {
		return nil
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID == ids.Nil {
		return nil
	}
	host := ids.From[ids.UserKind](actor.UserID)
	return &host
}

// replayedActivity resolves the (source_system, source_id) idempotency
// key: replaying a capture returns the existing activity. The replay
// path returns a record, so it is a read and carries the read's row
// scope: replaying someone else's external key must not hand over their
// activity. Out of scope answers the same 409 the unique-index race
// does — the key is taken, the record is not disclosed.
// replayMovedTheMeeting applies the one thing a redelivery of the same natural
// key may legitimately change: how the meeting went.
//
// A replay is otherwise the same fact arriving twice and writes nothing — that
// is what makes capture idempotent, and it is why the subject a rep corrected
// and the body they cleaned up survive the provider sending its own version
// again. The status is different in kind: booked → held / no_show / canceled is
// a vocabulary that EXISTS to change over time, and nothing on this side of
// the connector can know it moved.
//
// Left untouched, the row went on rendering an upcoming meeting that had been
// cancelled, the call answered 200 saying so, and no activity.updated fired —
// so the lead ladder and everything else downstream never re-read it.
//
// Through updateActivityInTx rather than an UPDATE here, so the move takes the
// write lock, records the transition (once, because it changed), and carries
// the same bounded delta a human's PATCH does. A held row refuses this write
// like any other, which is the point of the hold — the capture reports the
// refusal rather than claiming a success it did not have.
//
// Moving the status changes a row that already exists, so it asks for the
// update grant a PATCH asks for. The create grant that admitted the log only
// covers a replay that changes nothing.

func replayedActivity(ctx context.Context, tx pgx.Tx, in LogActivityInput) (*crmcontracts.Activity, error) {
	if in.SourceSystem == nil || in.SourceID == nil {
		return nil, nil
	}
	return replayedSourceActivity(ctx, tx, *in.SourceSystem, *in.SourceID, orWallClock(in.recordedAt))
}

func replayedSourceActivity(ctx context.Context, tx pgx.Tx, source, key string, asOf time.Time) (*crmcontracts.Activity, error) {
	var existing ids.ActivityID
	err := tx.QueryRow(ctx,
		`SELECT id FROM activity
		   WHERE source_system = $1 AND source_id = $2`,
		source, key).Scan(&existing)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// The row exists — the SELECT above just found it — so the only way
	// readActivity's own row-scope gate can answer ErrNotFound here is that
	// the key belongs to someone out of scope.
	out, err := readActivityAt(ctx, tx, existing, storekit.IncludeArchived, asOf)
	if errors.Is(err, apperrors.ErrNotFound) {
		return nil, apperrors.ErrConflict
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

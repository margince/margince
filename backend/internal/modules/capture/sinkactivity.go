// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Landing ONE activity row: the upsert on the natural key, the links, the
// participants, and the audit and event that go with a row that is new.
//
// Only the timeline row itself. Orchestrating a whole record — the raw
// original, the gates that can drop a message, the counterparty ladder, the
// post-commit work — is sink.go.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// captureActivity lands one activity: upsert on the natural key, links,
// audit and event only when the row is new — a replay writes nothing.
func (s *Sink) captureActivity(ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord, fields ActivityFields) (datasource.EntityRef, bool, counterpartyDecision, error) {
	// One clock read for the whole capture. A provider payload carrying no
	// timestamp falls back to now(), and THREE things downstream ask for that
	// answer — the activity row, its audit image, and the reply fact — so asking
	// separately files one message under three different times, and the reply
	// event then claims to describe an activity it disagrees with. fields is a
	// value copy, so settling it here settles it for every one of them.
	fields.OccurredAt = defaultOccurredAt(fields.OccurredAt)
	// ONE decision per capture, taken before the row exists and carried to both
	// the insert and this seat's import row. Asking twice would not merely cost
	// two reads: the marker step RE-OPENS a settled thread verdict as it
	// decides, and running that write a second time is a second claim on a row
	// the first pass already moved.
	birth, err := decideBirthTx(ctx, tx, rec, fields)
	if err != nil {
		return datasource.EntityRef{}, false, counterpartyDecision{}, err
	}
	id, created, err := s.upsertActivity(ctx, tx, rec, fields, birth)
	if err != nil {
		return datasource.EntityRef{}, false, counterpartyDecision{}, err
	}
	ref := datasource.EntityRef{Type: datasource.EntityActivity, ID: id.UUID}
	if !created {
		// The message was already here, delivered by somebody else's mailbox —
		// and THIS mailbox delivering it too is a fact the row cannot hold,
		// because captured_by names one importer. Recording it before the
		// return is what makes the second seat a party to the message at all:
		// without these two rows they have no import row to carry their
		// posture or their verdict, and no participant row to read the message
		// through once it is held.
		//
		// The order is deliberate. The import row first, then the participants,
		// then the recompute over the set the first two just completed — a
		// recompute that ran before this seat's import row landed would derive
		// an audience from a contributor set missing exactly the seat whose
		// sync it is.
		if err := s.recordThisImport(ctx, tx, id, rec, fields, birth); err != nil {
			return datasource.EntityRef{}, false, counterpartyDecision{}, err
		}
		return ref, false, counterpartyDecision{}, nil
	}
	// Everything a NEW row still needs: its links, its files, its people, its
	// audit and event, and the ladder's decision about who it is with. Split out
	// so this function reads as the three answers a capture can have — the row
	// was already here, the row is new, or the capture failed.
	decision, err := s.finishNewActivity(ctx, tx, id, rec, fields, birth)
	if err != nil {
		return datasource.EntityRef{}, false, counterpartyDecision{}, err
	}
	return ref, true, decision, nil
}

// finishNewActivity completes an activity row this capture just minted. It runs
// only for a genuinely new row: a replay writes none of it, which is what makes
// an at-least-once sync loop cost no duplicate audit entries.
func (s *Sink) finishNewActivity(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID,
	rec connector.NormalizedRecord, fields ActivityFields, birth birthDecision,
) (counterpartyDecision, error) {
	if err := s.linkActivity(ctx, tx, id, rec.Links); err != nil {
		return counterpartyDecision{}, err
	}
	// The files, after the links: the account roll-up a captured file carries is
	// read from the activity's own organization link, which does not exist until
	// the line above has run.
	//
	// Staged HERE, inside the transaction and only once the message is known to
	// be new. The bytes still land before the row that points at them — the put
	// is not transactional — but two things now cannot happen. Colleague mail
	// the internal gate drops never has its files written at all, which it did
	// when staging ran ahead of that gate. And a replayed message writes no
	// second copy: every pull minted fresh keys and then skipped the insert, so
	// a routine backfill left an unreferenced object per attachment per pass.
	staged, err := s.stageParts(ctx, rec)
	if err != nil {
		return counterpartyDecision{}, err
	}
	if err := s.recordParts(ctx, tx, id, rec, fields, staged); err != nil {
		return counterpartyDecision{}, err
	}
	if err := s.logPartDrops(ctx, tx, rec); err != nil {
		return counterpartyDecision{}, err
	}
	// Who was in it (ACT-DDL-3). Stamped here, beside the links, because the
	// connector principal bound to THIS context is the only place the mailbox
	// owner is known — every consumer downstream sees an activity whose
	// captured_by reads `connector:gmail` and cannot recover the human behind
	// it. The participant rows are the record of that fact.
	if err := stampCaptureParticipants(ctx, tx, id, actorUserID(ctx), fields.Kind, fields.Direction, rec.Counterparty.Email); err != nil {
		return counterpartyDecision{}, err
	}
	// Everyone else who was in it — the CCs, the meeting's organizer and
	// attendees. Separate from the two ends above because these are resolved
	// against our own people here rather than promoted later.
	// The recipient list is OURS to trust only when the provider attested our
	// own mailbox owner sent this message — then the Cc line is what our user
	// typed. On anything inbound it is the sender's text.
	if err := StampFurtherParticipants(ctx, tx, id, fields.Kind,
		rec.Counterparty.SentByOwner(), rec.Participants); err != nil {
		return counterpartyDecision{}, err
	}
	// And the names those rows just recorded, for an attendee who is ALREADY a
	// contact. A calendar invitation names every attendee in full, and that is
	// the only full name a contact minted from a bare address ever gets: the
	// ladder that names people never runs for a meeting, because attendance is
	// a list and the mapper leaves the counterparty unset. The other ordering —
	// an attendee who becomes a contact later — belongs to the cohort repair.
	if s.nameParticipants != nil && namesSomebody(rec.Participants) {
		if err := s.nameParticipants(ctx, tx, id); err != nil {
			return counterpartyDecision{}, err
		}
	}
	// Capture-audit minimization (ADR-0072/A118): the after-image is
	// metadata-only, never the subject/body (capturedActivityAuditImage).
	auditID, err := storekit.Audit(ctx, tx, "create", "activity", id.UUID, nil, capturedActivityAuditImage(rec, fields))
	if err != nil {
		return counterpartyDecision{}, err
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, id.UUID, activityCaptureEventPayload(fields.Kind, fields.ChannelProvider, rec.NaturalKey.SourceSystem)); err != nil {
		return counterpartyDecision{}, err
	}
	if err := s.emitReply(ctx, tx, auditID, id, rec, fields); err != nil {
		return counterpartyDecision{}, err
	}
	// The tiered creation gate decides and records in THIS transaction, so a
	// SUCCESSFUL gate leaves no window between an activity landing and its
	// disposition being known. A gate FAULT is contained by the savepoint inside
	// decideCounterpartyGuarded: it costs the derivation only, the message still
	// commits, and the link-less activity plus its capture_ensure_fault
	// breadcrumb are what the reconcile pass looks for. Failing the whole capture
	// would throw away a message we had already successfully read.
	decision, err := s.decideCounterpartyGuarded(ctx, tx, rec, id.UUID)
	if err != nil {
		return counterpartyDecision{}, err
	}
	// A meeting names no counterparty, so the gate above created nothing and
	// nothing has filed this row anywhere. The people who were in it are already
	// resolved on the participant rows, so the links come from there — BEFORE
	// the audience limiter, which decides what a link-less record is born as.
	derivedLinks := 0
	if fields.Kind == meetingKind {
		derivedLinks, err = s.linkResolvedMeetingParticipants(ctx, tx, id, len(rec.Links))
		if err != nil {
			return counterpartyDecision{}, err
		}
	}
	if err := limitLinkLessAudience(ctx, tx, id, rec, fields.Kind, decision, derivedLinks); err != nil {
		return counterpartyDecision{}, err
	}
	// This mailbox's own record of having imported the message, and the
	// recompute over it. After limitLinkLessAudience, because that decides the
	// audience a link-less message is BORN with and the recompute must derive
	// from the state the capture actually settled on rather than from the one it
	// held mid-transaction.
	if err := s.recordThisImport(ctx, tx, id, rec, fields, birth); err != nil {
		return counterpartyDecision{}, err
	}
	// The trace runs LAST, so it can carry the reason the ladder just settled on:
	// a message from a sender a previous verdict judged noise commits exactly
	// like any other and is then archived by the hide sweep, and a trace saying
	// only "captured" would answer "why did this not appear" with "it did".
	if err := s.traceActivity(ctx, tx, rec, id.UUID, decision); err != nil {
		return counterpartyDecision{}, err
	}
	return decision, nil
}

// activityCaptureEventPayload builds the activity.captured event for the
// capture ingestion path — the one emit site (of the event's two) that
// names an originating source system; the direct-log path
// (activities/activity.go) sets no fields but kind.
func activityCaptureEventPayload(kind, channelProvider, sourceSystem string) crmcontracts.PublicEventActivityCaptured {
	p := crmcontracts.PublicEventActivityCaptured{Kind: kind, SourceSystem: &sourceSystem}
	// Present only for a message, matching the envelope's own rule. This is the
	// path that carries every inbound channel message, so a consumer that could
	// once read the transport off the kind reads it here instead (ADR-0107/A158).
	if channelProvider != "" {
		p.ChannelProvider = &channelProvider
	}
	return p
}

func (s *Sink) upsertActivity(
	ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord,
	fields ActivityFields, birth birthDecision,
) (ids.ActivityID, bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return ids.ActivityID{}, false, err
	}
	occurredAt := fields.OccurredAt
	audience, audienceReason := birth.bornAudience()
	var id ids.ActivityID
	err := tx.QueryRow(ctx, `
		INSERT INTO activity (kind, channel_provider, subject, body, occurred_at, direction, source_system, source_id, source, captured_by, thread_key, counterparty_email, counterparty_outbound_attested, bulk_mail_attested, audience, audience_reason, has_calendar_part)
		VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), NULLIF($4, ''), $5, NULLIF($6, ''), $7, $8, $9, $10, NULLIF($11, ''), NULLIF($12, ''), $13, $14, $15, NULLIF($16, ''), $17)
		ON CONFLICT (source_system, source_id) WHERE source_system IS NOT NULL AND source_id IS NOT NULL
		DO NOTHING
		RETURNING id`,
		// NULLIF on channel_provider, not the empty string: the column FKs into
		// channel_provider, and '' names no provider — so a non-channel record
		// has to store NULL or the insert fails the foreign key.
		fields.Kind, fields.ChannelProvider, fields.Subject, fields.Body, occurredAt, fields.Direction,
		rec.NaturalKey.SourceSystem, rec.NaturalKey.SourceID, captureSource(rec), capturedByFor(ctx, rec), rec.ThreadKey,
		// Normalized lowercased at the write (a connector need not lowercase the
		// header case), matching the person_email normalization, so the T1
		// correspondence lookup's index-backed equality matches regardless of
		// the sender's casing without a runtime case fold.
		strings.ToLower(strings.TrimSpace(rec.Counterparty.Email)),
		// The provider's filing AND the message's authorship, never the
		// From-derived direction alone: this column is the T1
		// correspondence-positive gate's only evidence, and a forged
		// From:owner must not register as the owner's correspondence.
		rec.Counterparty.SentByOwner(),
		// This message's own RFC 2369 List-Unsubscribe header — the corroboration
		// a noise REDACTION needs before it destroys content. Stamped per
		// message, so a newsletter blast is destroyable while a personal mail
		// from the same address is only ever hidden.
		rec.Counterparty.ListUnsubscribe,
		audience, audienceReason,
		// What the parser read, stored as read. A record that carried no
		// calendar part stores false rather than NULL: NULL is reserved for the
		// rows captured before this column existed, so the two stay tellable
		// apart.
		fields.HasCalendarPart).Scan(&id)
	if err == nil {
		// Field-level provenance (B-E02.12) for the content fields this
		// capture set — same source/author the row itself carries.
		var stamps []storekit.FieldStamp
		for _, f := range []struct{ field, value string }{
			{"subject", fields.Subject}, {"body", fields.Body}, {"direction", fields.Direction},
		} {
			if f.value != "" {
				stamps = append(stamps, storekit.FieldStamp{Field: f.field})
			}
		}
		if err := storekit.StampFields(ctx, tx, "activity", id.UUID, captureSource(rec), rec.CapturedBy, stamps); err != nil {
			return ids.ActivityID{}, false, err
		}
		return id, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ids.ActivityID{}, false, fmt.Errorf("capture: activity upsert: %w", err)
	}
	// Replay: the natural key already landed — return the incumbent. Returning
	// a record is a read, so the row scope binds on this path too; an activity
	// scopes through its links, which can move after the first capture.
	err = tx.QueryRow(ctx,
		`SELECT id FROM activity WHERE source_system = $1 AND source_id = $2`,
		rec.NaturalKey.SourceSystem, rec.NaturalKey.SourceID).Scan(&id)
	if err != nil {
		return ids.ActivityID{}, false, fmt.Errorf("capture: activity replay lookup: %w", err)
	}
	if err := auth.EnsureActivityVisible(ctx, tx, id.UUID); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return ids.ActivityID{}, false, skipInvisibleIncumbent(rec, "activity")
		}
		return ids.ActivityID{}, false, err
	}
	return id, false, nil
}

// linkActivity resolves the normalized record's link refs. Every target
// is an FK argument naming a row-scoped record, so every one passes the
// visibility probe (H1) — a connector cannot plant a link to a row its
// granting human could not see.
func (s *Sink) linkActivity(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, links []datasource.EntityRef) error {
	for _, link := range links {
		column, ok := map[datasource.EntityType]string{
			datasource.EntityPerson:       "person_id",
			datasource.EntityOrganization: "organization_id",
			datasource.EntityDeal:         "deal_id",
		}[link.Type]
		if !ok {
			return fmt.Errorf("capture: activities cannot link a %s", link.Type)
		}
		if err := auth.EnsureLinkTarget(ctx, tx, string(link.Type), link.ID); err != nil {
			return fmt.Errorf("capture: link target %s %s: %w", link.Type, link.ID, err)
		}
		if _, err := tx.Exec(ctx, fmt.Sprintf(`
			INSERT INTO activity_link (activity_id, entity_type, %s)
			VALUES ($1, $2, $3)`, column),
			activityID, string(link.Type), link.ID); err != nil {
			return fmt.Errorf("capture: linking activity: %w", err)
		}
	}
	return nil
}

// defaultOccurredAt fills a provider payload that carried no timestamp:
// capture time is the honest fallback — better a coarse "when we saw
// it" than a zero time sorting the record to the beginning of history.
func defaultOccurredAt(occurredAt time.Time) time.Time {
	if occurredAt.IsZero() {
		return time.Now().UTC()
	}
	return occurredAt
}

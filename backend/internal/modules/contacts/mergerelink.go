// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Re-homing everything that points at the merged-away contact.
//
// Split from merge.go because it answers a different question: merge.go
// decides WHICH record survives and what the survivor inherits, and this
// decides where every satellite row now lives. A satellite this misses is not
// a broken merge — it is a row still pointing at a record no read returns,
// which is worse, because nothing fails and the data is simply gone from view.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// relinkContactReferences re-homes everything that points at the source
// contact — emails and phones (primaries demote when the survivor holds
// the slot), relationship edges, the pure link tables, consent (the
// restrictive rule), the lead promotion pointer, and the merge redirect
// chain — and returns the accounting the contact.merged event carries.
//
//nolint:cyclop // the rename added no branch: this body is what it was under the old noun.
func relinkContactReferences(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.ContactID) (relinkCounts, error) {
	counts := relinkCounts{}
	var err error
	if counts.Emails, err = relinkDemotingPrimary(ctx, tx, `
		UPDATE contact_email a SET contact_id = $2,
		  is_primary = a.is_primary AND NOT EXISTS (
		    SELECT 1 FROM contact_email b
		    WHERE b.contact_id = $2 AND b.email_type = a.email_type
		      AND b.is_primary AND b.archived_at IS NULL)
		WHERE a.contact_id = $1 AND a.archived_at IS NULL`, sourceID.UUID, targetID.UUID); err != nil {
		return counts, fmt.Errorf("relink emails: %w", err)
	}
	if counts.Phones, err = relinkDemotingPrimary(ctx, tx, `
		UPDATE contact_phone a SET contact_id = $2,
		  is_primary = a.is_primary AND NOT EXISTS (
		    SELECT 1 FROM contact_phone b
		    WHERE b.contact_id = $2 AND b.phone_type = a.phone_type
		      AND b.is_primary AND b.archived_at IS NULL)
		WHERE a.contact_id = $1 AND a.archived_at IS NULL`, sourceID.UUID, targetID.UUID); err != nil {
		return counts, fmt.Errorf("relink phones: %w", err)
	}
	// The enrichment sidecar moves with the contact. Left behind it is
	// invisible to every read of the survivor, so the evidence for their title
	// would vanish at a merge nobody expected to lose it — and the row would
	// then outlive the merged-away record's own archival.
	//
	// The one write of this table that is not writeContactProfileField, because it is
	// not a fill: it re-homes rows that already exist, whole and unexamined,
	// and it moves ALL of a contact's fields in one statement rather than
	// deciding one. The precedence is the same rule stated there — a
	// merged-away copy claims a field the survivor has not answered and never
	// replaces one — and the conflict target NAMES the uniqueness it relies
	// on, so a future constraint on this table cannot silently start swallowing
	// a different collision here.
	// observed_at and the superseded_* buffer travel WITH the row. Re-homing is
	// not a new statement, and letting the column take its default would date a
	// two-year-old signature as today — the merged copy would then outrank the
	// contact's own later mail, and a dropped buffer leaves a replaced value
	// nobody can put back.
	if _, err = tx.Exec(ctx, `
		INSERT INTO contact_profile_field
		  (contact_id, field, value, evidence_snippet, source_ref, confidence, source, captured_by,
		   observed_at, superseded_value, superseded_captured_by, superseded_observed_at)
		SELECT $2, field, value, evidence_snippet, source_ref, confidence, source, captured_by,
		       observed_at, superseded_value, superseded_captured_by, superseded_observed_at
		  FROM contact_profile_field WHERE contact_id = $1
		ON CONFLICT (contact_id, field) DO NOTHING`, sourceID.UUID, targetID.UUID); err != nil {
		return counts, fmt.Errorf("relink enrichment fields: %w", err)
	}
	if _, err = tx.Exec(ctx,
		`DELETE FROM contact_profile_field WHERE contact_id = $1`, sourceID.UUID); err != nil {
		return counts, fmt.Errorf("retire the merged-away enrichment fields: %w", err)
	}
	if err = relinkProviderPurchases(ctx, tx, sourceID, targetID); err != nil {
		return counts, err
	}
	if counts.Relationships, err = relinkContactEdges(ctx, tx, sourceID, targetID); err != nil {
		return counts, fmt.Errorf("relink relationships: %w", err)
	}
	if counts.ActivityLinks, err = relinkLinkRows(ctx, tx, "contact", sourceID.UUID, targetID.UUID); err != nil {
		return counts, fmt.Errorf("relink activity/list/tag rows: %w", err)
	}
	if err := relinkParticipantRows(ctx, tx, sourceID, targetID); err != nil {
		return counts, fmt.Errorf("relink activity participants: %w", err)
	}
	if err := mergeContactSocial(ctx, tx, sourceID, targetID); err != nil {
		return counts, fmt.Errorf("merge social rows: %w", err)
	}
	if err := mergeConsent(ctx, tx, sourceID, targetID); err != nil {
		return counts, fmt.Errorf("merge consent: %w", err)
	}
	// The channel identities follow the survivor, or the human behind them
	// keeps writing into the record nobody reads any more. No survivor-wins
	// rule is needed here, unlike the email and phone slots above: the unique
	// key spans (provider, channel_user_id) WITHOUT contact_id, so the two
	// halves cannot both hold the same identity live and the relink cannot
	// collide.
	if _, err := tx.Exec(ctx, `
		UPDATE contact_channel_identity SET contact_id = $2
		WHERE contact_id = $1 AND archived_at IS NULL`, sourceID, targetID); err != nil {
		return counts, fmt.Errorf("relink channel identities: %w", err)
	}
	// Why each contact existed follows the survivor, and both accounts are
	// kept rather than one winning. Two records merging is two accounts of how
	// this contact was obtained — one wrote first, the other was imported — and
	// the merged file is honestly the sum of them.
	//
	// Moved rather than copied, which matters for erasure: a row left on the
	// merged-away contact would be deleted by an erasure of THAT id and missed
	// by an erasure of the survivor, so the evidence would outlive whichever
	// request came second. Every satellite in this function moves for the same
	// reason.
	if err := relinkAcquisitionAndDuty(ctx, tx, sourceID, targetID); err != nil {
		return counts, err
	}
	// The promotion outcome pointer follows the survivor so a
	// re-promote 409 names a live contact.
	if _, err := tx.Exec(ctx,
		`UPDATE lead SET promoted_contact_id = $2 WHERE promoted_contact_id = $1`,
		sourceID, targetID); err != nil {
		return counts, fmt.Errorf("repoint lead promotions: %w", err)
	}
	// What the merged-away contact sent back through their own confirm link —
	// a correction they typed, or a request to be removed. It moves onto the
	// survivor because it is a request the workspace still owes an answer to,
	// and a rep reading the survivor's queue is the only reader there now is.
	// Left behind it hangs off a record no read returns, which is a data
	// subject's request quietly dropped.
	if _, err := tx.Exec(ctx,
		`UPDATE contact_confirm_submission SET contact_id = $2 WHERE contact_id = $1`,
		sourceID, targetID); err != nil {
		return counts, fmt.Errorf("relink confirm-page submissions: %w", err)
	}
	// Earlier merged-away rows repoint too: the redirect chain stays
	// one hop deep, so following merged_into_id always lands live.
	if _, err := tx.Exec(ctx,
		`UPDATE contact SET merged_into_id = $2 WHERE merged_into_id = $1`,
		sourceID, targetID); err != nil {
		return counts, fmt.Errorf("repoint earlier merges: %w", err)
	}
	return counts, nil
}

// readContactMergeState loads one end of a contact merge: a live row
// returns itself; an archived one returns its redirect pointer (nil when
// it was plain-archived, not merged). readCompanyMergeState (merge_company.go)
// is its company twin.
func readContactMergeState(ctx context.Context, tx pgx.Tx, id ids.ContactID) (crmcontracts.Contact, *ids.UUID, error) {
	// A merge-state read feeds the resolution decision, never the wire —
	// core columns suffice.
	p, err := readContact(ctx, tx, id, storekit.IncludeArchived, nil)
	if err != nil {
		return crmcontracts.Contact{}, nil, err
	}
	if p.ArchivedAt == nil {
		return p, nil, nil
	}
	return crmcontracts.Contact{}, (*ids.UUID)(p.MergedIntoId), apperrors.ErrNotFound
}

// mergePair resolves and validates both ends. A merge CHANGES both of them —
// the source is archived onto the survivor, the survivor absorbs it — so both
// ends carry the write-authority probe rather than the visibility one: a
// colleague handed a `read` share of either record may not spend it on a merge.
//
// The source must be live and writable; a source that was already merged away
// answers 409 with the pointer (the caller just proved they can address the
// row, so the outcome discloses nothing new — the AlreadyPromoted precedent).
// The target must be live too, and a target the caller cannot change answers a
// bare conflict rather than naming itself, exactly as an out-of-scope one does:
// merging returns the survivor, so the refusal must disclose no more than the
// caller could already read. An archived target can survive nothing.
func mergePair[T any, K ids.EntityKind](ctx context.Context, tx pgx.Tx, kind string, sourceID, targetID ids.ID[K],
	read func(context.Context, pgx.Tx, ids.ID[K]) (T, *ids.UUID, error),
) (source, target T, err error) {
	var zero T
	if err := auth.EnsureWritable(ctx, tx, kind, sourceID.UUID); err != nil {
		return zero, zero, err
	}
	source, mergedInto, err := read(ctx, tx, sourceID)
	if err != nil {
		if mergedInto != nil && !mergedInto.IsZero() {
			return zero, zero, &AlreadyMergedError{Kind: kind, IntoID: *mergedInto}
		}
		return zero, zero, err
	}

	writable, err := auth.WritableBy(ctx, tx, kind, targetID.UUID)
	if err != nil {
		return zero, zero, err
	}
	if !writable {
		return zero, zero, apperrors.ErrConflict
	}
	target, _, err = read(ctx, tx, targetID)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return zero, zero, &MergedTargetError{Kind: kind}
		}
		return zero, zero, err
	}
	return source, target, nil
}

// relinkDemotingPrimary runs a relink UPDATE whose SET clause demotes the
// row's primary flag when the survivor already fills that primary slot.
func relinkDemotingPrimary(ctx context.Context, tx pgx.Tx, stmt string, sourceID, targetID ids.UUID) (int64, error) {
	tag, err := tx.Exec(ctx, stmt, sourceID, targetID)
	return tag.RowsAffected(), err
}

// relinkContactEdges moves A's relationship edges to B: duplicates of an
// edge B already has archive, the rest relink with the current-primary
// employer flag demoted when B already has one.

// mergeContactSocial re-homes A's social rows onto B: a platform the
// survivor already has keeps B's handle and drops A's (same
// survivor-wins rule as the primary-slot demotions), the rest relink.
func mergeContactSocial(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.ContactID) error {
	if _, err := tx.Exec(ctx, `
		DELETE FROM contact_social a
		WHERE a.contact_id = $1 AND EXISTS (
		  SELECT 1 FROM contact_social b
		  WHERE b.contact_id = $2 AND b.platform = a.platform)`,
		sourceID, targetID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx,
		`UPDATE contact_social SET contact_id = $2 WHERE contact_id = $1`, sourceID, targetID)
	return err
}

// relinkProviderPurchases moves the merged-away record's purchased provider
// data onto the survivor: the claims, and the runs that bought them.
//
// Claims relink outright. They are keyed UNIQUE(run_id, claim_key), so two
// runs' answers about what is now one human coexist as peer assertions —
// which is the point: both sides were PAID for, and a merge that dropped one
// would throw away data the customer bought (PI-AC-11).
//
// Runs are the harder half, because at most one LIVE run may exist per
// (contact, provider, fingerprint) and a merge can bring two together. The
// rule is that nothing which may already have been charged is discarded:
//
//   - a run still `queued` never reached the provider, so cancelling it costs
//     nothing and the survivor's own run buys the same data;
//   - a run past `queued` may have been paid for, so it keeps its reservation
//     and runs to its own terminal state. When both sides are past `queued`
//     the source's run is re-fingerprinted rather than cancelled: that takes
//     it out of the live-run index — the established idiom, the same one
//     markSkipped uses — while leaving its money and its outcome intact.
func relinkProviderPurchases(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.ContactID) error {
	if _, err := tx.Exec(ctx,
		`UPDATE contact_provider_claim SET contact_id = $2 WHERE contact_id = $1`,
		sourceID.UUID, targetID.UUID); err != nil {
		return fmt.Errorf("relink provider claims: %w", err)
	}
	// What those purchases filled moves with them. Left on the loser, the rows
	// would name a record nobody reads while the survivor carries the values
	// they describe — so a later revert would clear nothing and the erasure
	// would have to find them through a contact the merge retired.
	if _, err := tx.Exec(ctx,
		`UPDATE provider_applied_field SET contact_id = $2 WHERE contact_id = $1`,
		sourceID.UUID, targetID.UUID); err != nil {
		return fmt.Errorf("relink what those purchases filled: %w", err)
	}
	// The source's queued runs are cancelled unconditionally. Not because the
	// survivor will buy the same data — it may hold no run at all, or one
	// parked in submission_unknown that never delivers — but because the
	// merged-away record has stopped being a subject: nothing left the
	// building for it, nothing was charged, and a run that would enrich a row
	// no read returns is work nobody wants. A human who still wants that
	// purchase asks for it on the survivor.
	if _, err := tx.Exec(ctx, `
		UPDATE provider_run
		   SET state = 'cancelled', completed_at = now(),
		       input_fingerprint = 'merged:' || gen_random_uuid()::text
		 WHERE contact_id = $1 AND state = 'queued'`, sourceID.UUID); err != nil {
		return fmt.Errorf("cancel the merged-away record's queued runs: %w", err)
	}
	// A survivor run that is still queued loses to a source run that already
	// left the building: the source's may have been charged, the survivor's
	// certainly has not.
	if _, err := tx.Exec(ctx, `
		UPDATE provider_run s
		   SET state = 'cancelled', completed_at = now(),
		       input_fingerprint = 'merged:' || gen_random_uuid()::text
		 WHERE s.contact_id = $2 AND s.state = 'queued'
		   AND EXISTS (
		     SELECT 1 FROM provider_run o
		      WHERE o.contact_id = $1 AND o.provider = s.provider
		        AND o.input_fingerprint = s.input_fingerprint
		        AND o.state IN ('submitting','in_progress','submission_unknown'))`,
		sourceID.UUID, targetID.UUID); err != nil {
		return fmt.Errorf("cancel the survivor's unspent colliding runs: %w", err)
	}
	// Both sides live and past queued: keep both, and take the source's out
	// of the live-run index so the relink below cannot violate it. Its
	// reservation and its terminal state are untouched.
	//
	// The survivor set is the past-queued states only. A survivor still
	// `queued` cannot appear here — the statement above already cancelled
	// every one of those that collides with a source run in exactly these
	// states — so naming `queued` would be an arm that can never fire.
	if _, err := tx.Exec(ctx, `
		UPDATE provider_run s
		   SET input_fingerprint = 'merged:' || gen_random_uuid()::text
		 WHERE s.contact_id = $1
		   AND s.state IN ('submitting','in_progress','submission_unknown')
		   AND EXISTS (
		     SELECT 1 FROM provider_run o
		      WHERE o.contact_id = $2 AND o.provider = s.provider
		        AND o.input_fingerprint = s.input_fingerprint
		        AND o.state IN ('submitting','in_progress','submission_unknown'))`,
		sourceID.UUID, targetID.UUID); err != nil {
		return fmt.Errorf("re-fingerprint the merged-away record's live runs: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE provider_run SET contact_id = $2 WHERE contact_id = $1`,
		sourceID.UUID, targetID.UUID); err != nil {
		return fmt.Errorf("relink provider runs: %w", err)
	}
	return nil
}

// relinkParticipantRows moves who-was-in-it onto the survivor.
//
// activity_link says how a message is FILED and moves with the rest of the
// satellites above; activity_participant says who was IN it, and it used to stay
// behind. The result was a merge that looked complete and left the interaction
// graph — "who on our team knows this contact" — crediting a record no read
// returns, because every reader of those rows matches contact_id exactly and none
// walks merged_into_id.
//
// Delete before repoint, because uq_activity_participant spans
// (activity, role, user, contact, address, channel_user_id): where BOTH halves of
// the merge sat on one activity in the same role and with the same other arms,
// they are one party recorded twice, so the source's row is dropped rather than
// collided into the survivor's. Two rows differing by address are NOT that case
// — they are two addresses of one party — and both repoint, which is what
// capture would have written had the records been one all along. Two differing
// by ACCOUNT are the same story in the chat vocabulary, which is why the twin
// test carries that arm too: without it a merge silently drops one of the two
// group chats a contact was on.
func relinkParticipantRows(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.ContactID) error {
	if _, err := tx.Exec(ctx, `
		DELETE FROM activity_participant ap
		 WHERE ap.contact_id = $1
		   AND EXISTS (
		       SELECT 1 FROM activity_participant twin
		        WHERE twin.activity_id = ap.activity_id
		          AND twin.role = ap.role
		          AND twin.contact_id = $2
		          AND coalesce(twin.user_id, '00000000-0000-0000-0000-000000000000')
		            = coalesce(ap.user_id, '00000000-0000-0000-0000-000000000000')
		          AND coalesce(twin.address, '') = coalesce(ap.address, '')
		          AND coalesce(twin.channel_user_id, '') = coalesce(ap.channel_user_id, ''))`,
		sourceID, targetID); err != nil {
		return fmt.Errorf("drop the duplicated participant rows: %w", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE activity_participant SET contact_id = $2 WHERE contact_id = $1`,
		sourceID, targetID); err != nil {
		return fmt.Errorf("repoint the participant rows: %w", err)
	}
	return nil
}

// relinkAcquisitionAndDuty moves why the contact exists, and what that obliged,
// onto the survivor.
//
// One function because they are one fact over two tables: the evidence says how
// the contact arrived and the case says what the installation owes for it, and
// a case left on the retired record is a disclosure still owed that the
// survivor's queue can no longer see. In this file because the lifecycle census
// (gates/satellite_lifecycle_test.go) reads this FILE's SQL literals to prove
// every satellite joins the merge path.
func relinkAcquisitionAndDuty(ctx context.Context, tx pgx.Tx, sourceID, targetID ids.ContactID) error {
	if _, err := tx.Exec(ctx, `
		UPDATE contact_acquisition_evidence SET contact_id = $2
		WHERE contact_id = $1`, sourceID, targetID); err != nil {
		return fmt.Errorf("relink acquisition evidence: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE privacy_notice_case SET contact_id = $2, updated_at = now()
		WHERE contact_id = $1`, sourceID, targetID); err != nil {
		return fmt.Errorf("relink notice cases: %w", err)
	}
	return nil
}

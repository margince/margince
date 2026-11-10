// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The split step of meetingSeriesSlotsDataTask. A meeting folded onto a row
// that holds another slot of its series is made its own meeting again. That
// happens only where unarchiving it gives back what the fold took.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/platform/originalstore"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// splitFoldedOccurrencesBatch settles each meeting-copy fold the page names.
// A fold it may not split is logged for review with the reason.
func splitFoldedOccurrencesBatch(
	ctx context.Context, tx pgx.Tx, meetings *activities.Store, in datatask.BatchIn,
) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	occurrences, err := listStoredOccurrences(ctx, tx, foldedSeriesOriginalsSQL, in)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	scanned := make([]ids.UUID, 0, len(occurrences))
	split := 0
	for _, o := range occurrences {
		scanned = append(scanned, o.original)
		o.payload, err = originalstore.RepairPayload(ctx, o.payload)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		fold, found, err := meetingCopyFoldOf(ctx, tx, o)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if !found {
			continue
		}
		done, refusal, err := settleFold(ctx, tx, meetings, fold)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if refusal != "" {
			slog.WarnContext(ctx, "compose: a folded meeting pair is left for review",
				"meeting", fold.survivor.meeting.String(), "folded", fold.folded.meeting.String(), "reason", refusal)
		}
		if done {
			split++
		}
	}
	return datatask.AdvanceUUID(in, scanned, split)
}

// occurrenceFold is a meeting a meeting-copy fold archived, and the row it
// was folded into.
type occurrenceFold struct {
	folded, survivor         storedOccurrence
	foldedSlot, survivorSlot seriesSlot
	archive                  ids.UUID
	evidence                 []byte
	foldedAt                 time.Time
}

// meetingCopyFoldOf answers the fold behind o's latest archive, or false when
// that archive was no meeting-copy fold.
func meetingCopyFoldOf(ctx context.Context, tx pgx.Tx, o storedOccurrence) (occurrenceFold, bool, error) {
	fold := occurrenceFold{folded: o}
	archive, found, err := latestArchiveOf(ctx, tx, o.meeting)
	if err != nil || !found {
		return occurrenceFold{}, false, err
	}
	survivor, folded := activities.MeetingCopySurvivor(archive.evidence)
	if !folded {
		return occurrenceFold{}, false, nil
	}
	fold.archive, fold.evidence, fold.foldedAt = archive.id, archive.evidence, archive.at
	fold.survivor.meeting = survivor
	return fold, true, nil
}

type archiveEntry struct {
	id       ids.UUID
	evidence []byte
	at       time.Time
}

func latestArchiveOf(ctx context.Context, tx pgx.Tx, meeting ids.ActivityID) (archiveEntry, bool, error) {
	var entry archiveEntry
	err := tx.QueryRow(ctx, `
		SELECT id, evidence, occurred_at FROM audit_log
		 WHERE entity_type = 'activity' AND entity_id = $1 AND action = 'archive'
		 ORDER BY occurred_at DESC, id DESC LIMIT 1`, meeting).Scan(&entry.id, &entry.evidence, &entry.at)
	if errors.Is(err, pgx.ErrNoRows) {
		return archiveEntry{}, false, nil
	}
	if err != nil {
		return archiveEntry{}, false, fmt.Errorf("compose: reading why a meeting was archived: %w", err)
	}
	return entry, true, nil
}

// settleFold locks both rows, then judges and splits the fold on what the lock
// holds still. It reports whether it split the pair, and why not when the
// pair is left for review. A right fold of one occurrence is left silently.
func settleFold(ctx context.Context, tx pgx.Tx, meetings *activities.Store, fold occurrenceFold) (bool, string, error) {
	pair, err := lockFoldPairForSplit(ctx, tx, fold)
	if err != nil {
		return false, "", err
	}
	// Before stands: a restricted survivor is archived too, and is logged.
	if pair.restricted || pair.scrubbed {
		return false, "a row is erased, anonymized or restricted", nil
	}
	if !pair.stands {
		return false, "", nil
	}
	fold, wrong, err := withSlots(ctx, tx, fold)
	if err != nil || !wrong {
		return false, "", err
	}
	if !pair.sameSeat {
		return false, "the rows came from two seats' calendars", nil
	}
	guests, refusal, err := foldSplitRefusal(ctx, tx, fold)
	if err != nil || refusal != "" {
		return false, refusal, err
	}
	refusal, err = splitUnderSavepoint(ctx, tx, meetings, fold, guests)
	return err == nil && refusal == "", refusal, err
}

// lockedFoldPair is what the lock found on both rows.
type lockedFoldPair struct {
	// stands holds while the survivor is live and the fold is still the
	// folded row's latest archive.
	stands, restricted, scrubbed, sameSeat bool
}

// lockFoldPairForSplit locks both rows FOR UPDATE in id order, so every check
// after it and the split read one state. A scrub verb on either row, at any
// time, keeps the pair: a split must never restore what an erasure removed.
func lockFoldPairForSplit(ctx context.Context, tx pgx.Tx, fold occurrenceFold) (lockedFoldPair, error) {
	pairIDs := []ids.UUID{fold.survivor.meeting.UUID, fold.folded.meeting.UUID}
	rows, err := tx.Query(ctx, `
		SELECT id, archived_at IS NULL, restricted_at IS NOT NULL, `+auth.ConnectorSeatSQL("a")+`
		  FROM activity a WHERE id = ANY($1) ORDER BY id FOR UPDATE`, pairIDs)
	if err != nil {
		return lockedFoldPair{}, fmt.Errorf("compose: locking a folded meeting pair: %w", err)
	}
	type lockedRow struct {
		id               ids.ActivityID
		live, restricted bool
		seat             *string
	}
	locked, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (lockedRow, error) {
		var r lockedRow
		return r, row.Scan(&r.id, &r.live, &r.restricted, &r.seat)
	})
	if err != nil {
		return lockedFoldPair{}, fmt.Errorf("compose: locking a folded meeting pair: %w", err)
	}
	if len(locked) != 2 {
		return lockedFoldPair{}, nil
	}
	var pair lockedFoldPair
	survivorLive := false
	for _, r := range locked {
		pair.restricted = pair.restricted || r.restricted
		if r.id == fold.survivor.meeting {
			survivorLive = r.live
		}
	}
	pair.sameSeat = locked[0].seat != nil && locked[1].seat != nil && *locked[0].seat == *locked[1].seat
	latest, found, err := latestArchiveOf(ctx, tx, fold.folded.meeting)
	if err != nil {
		return lockedFoldPair{}, err
	}
	pair.stands = survivorLive && found && latest.id == fold.archive
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM audit_log
		                WHERE entity_type = 'activity' AND entity_id = ANY($1) AND action = ANY($2))`,
		pairIDs, privacy.ScrubVerbs()).Scan(&pair.scrubbed); err != nil {
		return lockedFoldPair{}, fmt.Errorf("compose: reading whether a folded meeting pair was erased: %w", err)
	}
	return pair, nil
}

// withSlots reads both rows' series slots, and reports whether the fold
// joined two slots of one series. Both rows are unrestricted by now.
func withSlots(ctx context.Context, tx pgx.Tx, fold occurrenceFold) (occurrenceFold, bool, error) {
	err := tx.QueryRow(ctx, `
		SELECT rc.id, s.source_system, s.captured_by, `+originalstore.SQL("rc")+`
		  FROM activity s
		  JOIN raw_capture rc ON rc.source_system = s.source_system AND rc.source_id = s.source_id
		 WHERE s.id = $1 AND s.kind = 'meeting' AND s.restricted_at IS NULL AND s.source_system = ANY($2)`,
		fold.survivor.meeting, seriesCalendars).
		Scan(&fold.survivor.original, &fold.survivor.source, &fold.survivor.capturedBy, &fold.survivor.payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return occurrenceFold{}, false, nil
	}
	if err != nil {
		return occurrenceFold{}, false, fmt.Errorf("compose: reading the meeting a copy folded into: %w", err)
	}
	fold.survivor.payload, err = originalstore.RepairPayload(ctx, fold.survivor.payload)
	if err != nil {
		return occurrenceFold{}, false, err
	}
	foldedSlot, foldedInSeries := slotOf(fold.folded.source, fold.folded.payload)
	survivorSlot, survivorInSeries := slotOf(fold.survivor.source, fold.survivor.payload)
	if !foldedInSeries || !survivorInSeries || survivorSlot.series != foldedSlot.series || survivorSlot.key == foldedSlot.key {
		return occurrenceFold{}, false, nil
	}
	fold.foldedSlot, fold.survivorSlot = foldedSlot, survivorSlot
	return fold, true, nil
}

// foldSplitRefusal answers the guest rows the split takes back from the
// survivor, or why the split would not give back what the fold took.
func foldSplitRefusal(ctx context.Context, tx pgx.Tx, fold occurrenceFold) ([]ids.UUID, string, error) {
	var worked bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1
		                  AND actor_type = 'human' AND occurred_at >= $2)`,
		fold.survivor.meeting, fold.foldedAt).Scan(&worked); err != nil {
		return nil, "", fmt.Errorf("compose: reading whether a colleague changed a folded meeting: %w", err)
	}
	if worked {
		return nil, "a colleague changed the meeting after the fold", nil
	}
	handedOn, err := activities.MeetingCopyHandedOnDocumentsTx(ctx, tx, fold.folded.meeting, fold.survivor.meeting)
	if err != nil || handedOn {
		return nil, refusalIf(handedOn, "the fold moved document links"), err
	}
	guests, known, err := activities.MeetingCopyCarriedGuestsTx(ctx, tx,
		fold.folded.meeting, fold.survivor.meeting, fold.evidence, fold.foldedAt)
	if err != nil || !known {
		return nil, refusalIf(!known, "the fold's record does not say which guests it added"), err
	}
	holder, held, err := activities.ResolveIdentity(ctx, tx, activities.IdentityKindMeeting, fold.foldedSlot.key)
	if err != nil {
		return nil, "", err
	}
	if held && holder != fold.folded.meeting && holder != fold.survivor.meeting {
		return nil, "another meeting holds the folded occurrence's key", nil
	}
	return guests, "", nil
}

func refusalIf(refused bool, reason string) string {
	if refused {
		return reason
	}
	return ""
}

// splitUnderSavepoint runs the split, and rolls it back whole when a step
// after the restore refuses.
func splitUnderSavepoint(
	ctx context.Context, tx pgx.Tx, meetings *activities.Store, fold occurrenceFold, guests []ids.UUID,
) (string, error) {
	split, err := tx.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("compose: opening a meeting split: %w", err)
	}
	refusal, err := unfold(ctx, split, meetings, fold, guests)
	if err != nil || refusal != "" {
		if rollbackErr := split.Rollback(ctx); rollbackErr != nil {
			return "", errors.Join(err, rollbackErr)
		}
		return refusal, err
	}
	if err := split.Commit(ctx); err != nil {
		return "", fmt.Errorf("compose: finishing a meeting split: %w", err)
	}
	return "", nil
}

// unfold unarchives the folded row, takes back the guests the fold carried,
// and gives each row its own slot key.
func unfold(ctx context.Context, tx pgx.Tx, meetings *activities.Store, fold occurrenceFold, guests []ids.UUID) (string, error) {
	_, err := meetings.RestoreActivityTx(ctx, tx, fold.folded.meeting, nil, unarchiveWith(nil))
	if refusal, ok := errors.AsType[*storekit.RestoreRefusal](err); ok {
		return "the archive cannot be undone: " + string(refusal.Reason), nil
	}
	if errors.Is(err, apperrors.ErrConflict) || errors.Is(err, apperrors.ErrNotFound) {
		return "the archive cannot be undone", nil
	}
	if _, ok := errors.AsType[*activities.SchedulingArgumentError](err); ok {
		return "a calendar invitation holds the meeting", nil
	}
	if err != nil {
		return "", err
	}
	if err := activities.UndoMeetingCopyGuestsTx(ctx, tx, fold.folded.meeting, fold.survivor.meeting, guests); err != nil {
		return "", err
	}
	// The survivor's own claim retires the folded slot's key it was handed.
	if _, err := activities.RekeyMeetingIdentity(ctx, tx, fold.survivor.meeting, fold.survivorSlot.key, fold.survivor.capturedBy); err != nil {
		return "", err
	}
	if _, err := activities.RekeyMeetingIdentity(ctx, tx, fold.folded.meeting, fold.foldedSlot.key, fold.folded.capturedBy); err != nil {
		return "", err
	}
	holder, held, err := activities.ResolveIdentity(ctx, tx, activities.IdentityKindMeeting, fold.foldedSlot.key)
	if err != nil {
		return "", err
	}
	if !held || holder != fold.folded.meeting {
		return "another meeting holds the folded occurrence's key", nil
	}
	return "", activities.RecomputeAudiencesTx(ctx, tx, []ids.ActivityID{fold.survivor.meeting, fold.folded.meeting})
}

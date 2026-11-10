// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The original of a new message that carried files.
//
// Its files reach the object store before its original is written. So the
// original is written once, and names each object instead of carrying its
// bytes. Written whole, every file went through Postgres twice: once here, and
// once when the part sweep rewrote the row, leaving the first copy for vacuum.

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture/partslim"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type deferredOriginal struct{ pending bool }

type deferredOriginalKey struct{}

// deferOriginal holds back the original of a message whose files a keeper will
// store. A record whose connector stored its own original, or one with no
// files, keeps the original's place before the activity.
func (s *Sink) deferOriginal(ctx context.Context, rec connector.NormalizedRecord) (context.Context, bool) {
	if s.files == nil || len(rec.Parts) == 0 || len(rec.Raw) == 0 || !rec.StoredOriginal.IsZero() {
		return ctx, false
	}
	return context.WithValue(ctx, deferredOriginalKey{}, &deferredOriginal{pending: true}), true
}

func pendingOriginal(ctx context.Context) *deferredOriginal {
	held, ok := ctx.Value(deferredOriginalKey{}).(*deferredOriginal)
	if !ok || !held.pending {
		return nil
	}
	return held
}

// releaseHeldOriginalTx stores a held-back original whole, for a message that
// turned out not to be new, before anything records which original it read.
func releaseHeldOriginalTx(
	ctx context.Context, tx pgx.Tx, rec connector.NormalizedRecord,
) (connector.NormalizedRecord, error) {
	held := pendingOriginal(ctx)
	if held == nil {
		return rec, nil
	}
	held.pending = false
	return storeOriginalTx(ctx, tx, rec)
}

// releaseHeldOriginalForScopedKeyTx is the record a seat-scoped original is
// stored from. A held-back original reaches that path unfiltered. So a
// withholding message's file bytes come out here, as storeOriginalTx takes them
// out of the shared one.
func releaseHeldOriginalForScopedKeyTx(
	ctx context.Context, tx pgx.Tx, scoped connector.NormalizedRecord,
) (connector.NormalizedRecord, error) {
	held := pendingOriginal(ctx)
	if held == nil {
		return scoped, nil
	}
	held.pending = false
	return withholdOriginalPartsTx(ctx, tx, scoped)
}

// keepPartsAndOriginal stores a new activity's files, then its held-back
// original, which can name them only once they are stored.
func (s *Sink) keepPartsAndOriginal(
	ctx context.Context, tx pgx.Tx, activity ids.ActivityID, rec connector.NormalizedRecord, fields ActivityFields,
) (connector.NormalizedRecord, error) {
	kept, staged, err := s.keepParts(ctx, tx, activity, rec, fields)
	if err != nil {
		return kept, err
	}
	return kept, s.storeDeferredOriginalTx(ctx, tx, activity, rec, staged)
}

// storeDeferredOriginalTx writes a new activity's held-back original and points
// the activity at it. A message whose files were withheld or not stored keeps
// the bytes, as storeOriginalTx writes them.
func (s *Sink) storeDeferredOriginalTx(
	ctx context.Context, tx pgx.Tx, activity ids.ActivityID,
	original connector.NormalizedRecord, staged []StagedFile,
) error {
	held := pendingOriginal(ctx)
	if held == nil || !original.StoredOriginal.IsZero() {
		return nil
	}
	held.pending = false
	stored, err := s.storeSlimOriginalTx(ctx, tx, activity, original, staged)
	if err != nil {
		return err
	}
	if stored.IsZero() {
		whole, err := storeOriginalTx(ctx, tx, original)
		if err != nil {
			return err
		}
		stored = whole.StoredOriginal
	}
	if stored.IsZero() {
		return nil
	}
	if _, err := tx.Exec(ctx, `UPDATE activity SET raw_capture_id = $2 WHERE id = $1`, activity, stored); err != nil {
		return fmt.Errorf("capture: naming the stored original: %w", err)
	}
	return nil
}

// storeSlimOriginalTx stores the original without the bytes of the files this
// capture just stored, or answers nothing when no file can come out of it.
func (s *Sink) storeSlimOriginalTx(
	ctx context.Context, tx pgx.Tx, activity ids.ActivityID,
	original connector.NormalizedRecord, staged []StagedFile,
) (ids.UUID, error) {
	if len(staged) == 0 {
		return ids.Nil, nil
	}
	proved, err := s.filesThisCaptureStoredTx(ctx, tx, activity, original, staged)
	if err != nil || len(proved) == 0 {
		return ids.Nil, err
	}
	stripped, count, err := partslim.StripStoredParts(original.Raw, proved)
	if err != nil || count == 0 || len(stripped) >= len(original.Raw) {
		// The part sweep stamps an original it cannot rewrite; here the whole
		// original is stored instead, and the sweep meets it as it always has.
		return ids.Nil, nil //nolint:nilerr // an original that cannot be slimmed is stored whole
	}
	payload, err := rawCapturePayload(stripped)
	if err != nil {
		return ids.Nil, err
	}
	return insertOriginalRowTx(ctx, tx, original.NaturalKey, payload, outcomeSlimmed)
}

// filesThisCaptureStoredTx pairs each file this capture staged with the
// attachment row that now names its object. A file whose row already existed
// names an older object. It keeps its bytes in the original, and the part sweep
// proves them against that object.
func (s *Sink) filesThisCaptureStoredTx(
	ctx context.Context, tx pgx.Tx, activity ids.ActivityID,
	original connector.NormalizedRecord, staged []StagedFile,
) ([]partslim.StoredPart, error) {
	locations, err := s.files.Stored(staged)
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(locations))
	for _, location := range locations {
		keys = append(keys, location.StorageKey)
	}
	rows, err := tx.Query(ctx, `
		SELECT storage_key FROM attachment
		 WHERE activity_id = $1 AND storage_key = ANY($2::text[]) AND archived_at IS NULL AND NOT bytes_withheld`, activity, keys)
	if err != nil {
		return nil, fmt.Errorf("capture: reading the files this message stored: %w", err)
	}
	recorded, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("capture: reading the files this message stored: %w", err)
	}
	bodies := make(map[int][]byte, len(original.Parts))
	for _, part := range original.Parts {
		bodies[part.Ordinal] = part.Body
	}
	var proved []partslim.StoredPart
	for _, location := range locations {
		ordinal, ok := ordinalOf(location.PartID)
		if !ok || !slices.Contains(recorded, location.StorageKey) || bodies[ordinal] == nil {
			continue
		}
		proved = append(proved, partslim.StoredPart{
			Ordinal: ordinal, Sha256: location.Sha256, Bytes: location.Bytes,
			StorageKey: location.StorageKey, Body: bodies[ordinal],
		})
	}
	return proved, nil
}

// ordinalOf reads partIdentity back.
func ordinalOf(partID string) (int, bool) {
	digits, ok := strings.CutPrefix(partID, "part:")
	if !ok {
		return 0, false
	}
	ordinal, err := strconv.Atoi(digits)
	return ordinal, err == nil
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The files of personal mail captured before its thread was judged private.
//
// Capture keeps no bytes for a message on a thread already held as personal
// (sinkpersonalparts.go). The verdict usually lands after the thread's first
// messages, so those keep their files — and nothing destroys a personal THREAD
// the way the personal-mail purge destroys a personal SENDER's mail. This
// selects those messages once the same undo window the purge gives has closed,
// and rewrites their stored originals without the bytes.

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture/partslim"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// PrivateThreadMessage is one message whose files are due, and the stored
// original it was read from, if any.
type PrivateThreadMessage struct {
	Activity   ids.UUID
	RawCapture *ids.UUID
}

// SelectPrivateThreadFilesDueTx lists messages on a thread held as personal
// whose undo window has closed and that still have stored file bytes.
//
// The verdict is the capturing seat's own — the seat captured_by names, as the
// attendee repair reads it — and the message's author is one the verdict saw
// (senderWasSeen, at capture). The window is the purge's: a week
// when the owner held the thread, a month when the classifier did, measured
// from the later of the message's capture and the verdict. A message a
// colleague also imported is theirs too and is left alone. So, as the purge
// leaves them, is mail under a hold, inside the statutory floor, or named by an
// open data-subject request: withholding bytes cannot be undone either.
func SelectPrivateThreadFilesDueTx(
	ctx context.Context, tx pgx.Tx, windows PersonalPurgeWindows, floor StatutoryFloor, limit int,
) ([]PrivateThreadMessage, error) {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return nil, err
	}
	if limit <= 0 {
		return nil, nil
	}
	// Windows first, then the floor's own arguments, then the limit — each
	// placeholder derived from where its argument actually landed.
	args := []any{windows.ByOwner, windows.ByClassifier}
	shielded, args := floor.column(len(args), args)
	args = append(args, limit)
	limitAt := "$" + strconv.Itoa(len(args))
	rows, err := tx.Query(ctx, `
		SELECT a.id, a.raw_capture_id
		  FROM activity a
		  JOIN capture_thread_verdict v
		    ON v.thread_key = a.thread_key AND v.user_id::text = split_part(a.captured_by, ':', 3)
		 WHERE v.kind = 'personal' AND v.status IN ('held', 'held_by_owner')
		   AND v.resolved_at IS NOT NULL
		   AND a.counterparty_email = ANY (v.seen_addresses)
		   AND a.kind = 'email' AND a.captured_by LIKE 'connector:%'
		   AND a.archived_at IS NULL AND a.restricted_at IS NULL
		   AND NOT (`+shielded+`)
		   AND NOT `+underAnOpenRequest+`
		   AND greatest(a.created_at, v.resolved_at)
		       + (CASE WHEN v.status = 'held_by_owner' THEN $1 ELSE $2 END)::interval <= now()
		   AND NOT EXISTS (
		       SELECT 1 FROM capture_import o WHERE o.activity_id = a.id AND o.user_id <> v.user_id)
		   AND EXISTS (
		       SELECT 1 FROM attachment at
		        WHERE at.activity_id = a.id AND at.archived_at IS NULL
		          AND at.storage_key <> '' AND NOT at.bytes_withheld)
		 ORDER BY a.id
		 LIMIT `+limitAt, args...)
	if err != nil {
		return nil, fmt.Errorf("capture: selecting private-thread mail whose files are due: %w", err)
	}
	defer rows.Close()
	var out []PrivateThreadMessage
	for rows.Next() {
		var m PrivateThreadMessage
		if err := rows.Scan(&m.Activity, &m.RawCapture); err != nil {
			return nil, fmt.Errorf("capture: reading private-thread mail whose files are due: %w", err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("capture: reading private-thread mail whose files are due: %w", err)
	}
	return out, nil
}

// StoredBody is one stored file of a message, with its bytes: the ordinal the
// original numbers it by and the key a slimmed original refers to it under.
type StoredBody struct {
	Ordinal int
	Key     string
	Body    []byte
}

// WithholdStoredOriginalTx cuts these files' bytes out of the message's stored
// original — the third sanctioned rewrite of raw_capture, beside the part
// slim and erasure.
//
// A slimmed original no longer carries the bytes but names the object they
// moved to, so it is first restored from the bodies given, then withheld like
// any other. The row is stamped slimmed either way, so the slim sweep does not
// read it again.
func WithholdStoredOriginalTx(ctx context.Context, tx pgx.Tx, rawCaptureID ids.UUID, files []StoredBody) error {
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}
	var payload []byte
	err := tx.QueryRow(ctx,
		`SELECT payload FROM raw_capture WHERE id = $1 FOR UPDATE`, rawCaptureID).Scan(&payload)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("capture: reading the original to withhold its files: %w", err)
	}
	original, err := DecodeStoredOriginal(payload)
	if err != nil {
		return fmt.Errorf("capture: decoding the original to withhold its files: %w", err)
	}
	byKey := make(map[string][]byte, len(files))
	parts := make([]partslim.WithheldPart, 0, len(files))
	for _, f := range files {
		byKey[f.Key] = f.Body
		parts = append(parts, partslim.WithheldPart{Ordinal: f.Ordinal, Body: f.Body})
	}
	original, err = partslim.RestoreStoredParts(original, func(ref partslim.PartRef) ([]byte, error) {
		if body, ok := byKey[ref.StorageKey]; ok {
			return body, nil
		}
		return nil, fmt.Errorf("capture: the original names a part this message no longer stores: part:%d", ref.Ordinal)
	})
	if err != nil {
		// A stanza naming an object not among the files given cannot be put
		// back, so nothing of the body is trusted: only the headers are kept.
		original = partslim.HeadersOnly(original)
	} else {
		original, _ = partslim.WithholdParts(original, parts)
	}
	rewritten, err := EncodeStoredOriginal(original)
	if err != nil {
		return fmt.Errorf("capture: encoding the withheld original: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE raw_capture SET payload = $2::jsonb, parts_slimmed_at = now() WHERE id = $1`,
		rawCaptureID, rewritten); err != nil {
		return fmt.Errorf("capture: writing the withheld original: %w", err)
	}
	return nil
}

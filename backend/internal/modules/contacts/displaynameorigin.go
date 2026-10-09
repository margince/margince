// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Whose display name a contact carries: one capture read off a mail header or a
// calendar invitation, or one somebody chose. Only capture's guess is ever
// replaced by the name the record learned later. A name a human, an agent, an
// import or an API caller set is theirs, however it compares to the split
// columns.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The machine principals that mint a contact through the capture door after a
// sender has been judged. compose stamps its verdict and accept writes with
// these, so the origin test below and the writers cannot spell them apart.
const (
	CaptureVerdictActor = "agent:capture_counterparty_verdict"
	CaptureAcceptActor  = "agent:" + approvalsubject.KindCounterparty
)

// DisplayNameRepairActor is the principal the nightly participant_backfill job
// binds. Its display-name repairs are capture's own writes, so they never
// protect a name the way an explicit edit does.
const DisplayNameRepairActor = "system:participant_backfill"

// StaleDisplayNamesTx answers up to limit contacts whose display name the
// repair may replace with the pair their split columns carry. The repair's
// selector reads it, and RefreshDisplayNameTx asks the same statement about one
// contact. So a row the selector offers is one the writer accepts.
func StaleDisplayNamesTx(ctx context.Context, tx pgx.Tx, limit int) ([]ids.ContactID, error) {
	return captureGuessedDisplayNamesTx(ctx, tx, nil, nil, true, limit)
}

// displayNameIsStaleTx is StaleDisplayNamesTx asked about one contact.
func displayNameIsStaleTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID) (bool, error) {
	found, err := captureGuessedDisplayNamesTx(ctx, tx, &contactID, nil, true, 1)
	return len(found) == 1, err
}

// displayNameIsCapturesGuessTx answers whether the name fill may replace this
// contact's display name with the pair it is about to write. The split columns
// are still empty then, so the statement compares the display with that pair.
func displayNameIsCapturesGuessTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, pair ParsedName) (bool, error) {
	found, err := captureGuessedDisplayNamesTx(ctx, tx, &contactID, &pair, false, 1)
	return len(found) == 1, err
}

// captureGuessedDisplayNamesTx is the one statement that says whether a
// contact's display name may be replaced, for one contact (only) or for a page.
//
// Origin comes from captured_by, stamped once at create. Every capture sink
// runs as a `connector:` principal; the verdict and accept paths run as the two
// actors above. Every other door stamps something else and is left alone. Two
// connector doors are not capture: the vCard ingest copies a curated card, and
// an import names its system in source_system.
//
// Any later audit row naming full_name protects the name, unless capture or the
// repair wrote it. A human or an agent chose it, by update or by restore.
//
// The pair is the split columns, or the one the fill is about to write. A
// display containing both nonblank halves says at least as much as the pair,
// whoever wrote it. needPair is the repair's rule: it never writes one half.
func captureGuessedDisplayNamesTx(
	ctx context.Context, tx pgx.Tx, only *ids.ContactID, pair *ParsedName, needPair bool, limit int,
) ([]ids.ContactID, error) {
	var first, last *string
	if pair != nil {
		first, last = &pair.First, &pair.Last
	}
	rows, err := tx.Query(ctx, `
		SELECT p.id
		  FROM contact p
		 CROSS JOIN LATERAL (
		       SELECT btrim(coalesce($7::text, p.first_name, '')) AS first,
		              btrim(coalesce($8::text, p.last_name, '')) AS last) n
		 WHERE ($1::uuid IS NULL OR p.id = $1)
		   AND (p.captured_by LIKE 'connector:%' OR p.captured_by IN ($2, $3))
		   AND p.source IS DISTINCT FROM $4
		   AND p.source_system IS NULL
		   AND (NOT $5 OR (n.first <> '' AND n.last <> ''))
		   AND NOT (n.first <> '' AND n.last <> ''
		            AND strpos(lower(p.full_name), lower(n.first)) > 0
		            AND strpos(lower(p.full_name), lower(n.last)) > 0)
		   AND NOT EXISTS (
		       SELECT 1 FROM audit_log a
		        WHERE a.entity_type = 'contact' AND a.entity_id = p.id
		          AND a.action <> 'create'
		          AND (a.after ? 'full_name' OR a.before ? 'full_name')
		          AND a.actor_id NOT LIKE 'connector:%'
		          AND a.actor_id NOT IN ($2, $3, $9))
		 ORDER BY p.id
		 LIMIT $6`,
		only, CaptureVerdictActor, CaptureAcceptActor, vcardSource, needPair, limit,
		first, last, DisplayNameRepairActor)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading whose display name a contact carries: %w", err)
	}
	found, err := pgx.CollectRows(rows, pgx.RowTo[ids.ContactID])
	if err != nil {
		return nil, fmt.Errorf("contacts: reading whose display name a contact carries: %w", err)
	}
	return found, nil
}

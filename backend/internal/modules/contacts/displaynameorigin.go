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

// StaleDisplayNamesTx answers up to limit contacts whose display name the
// repair may replace with the pair their split columns carry. The repair's
// selector reads it, and RefreshDisplayNameTx asks the same statement about one
// contact. So a row the selector offers is one the writer accepts.
func StaleDisplayNamesTx(ctx context.Context, tx pgx.Tx, limit int) ([]ids.ContactID, error) {
	return captureGuessedDisplayNamesTx(ctx, tx, nil, true, limit)
}

// displayNameIsStaleTx is StaleDisplayNamesTx asked about one contact.
func displayNameIsStaleTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID) (bool, error) {
	found, err := captureGuessedDisplayNamesTx(ctx, tx, &contactID, true, 1)
	return len(found) == 1, err
}

// displayNameIsCapturesGuessTx answers whether this contact's display name is
// still capture's guess, so the name fill may move it with the split columns.
// The fill runs while both split columns are empty, so the pair test is off.
func displayNameIsCapturesGuessTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID) (bool, error) {
	found, err := captureGuessedDisplayNamesTx(ctx, tx, &contactID, false, 1)
	return len(found) == 1, err
}

// captureGuessedDisplayNamesTx is the one statement that says whose display
// name a contact carries, for one contact (only) or for a page of them.
//
// Origin comes from captured_by, stamped once at create. Every capture sink
// runs as a `connector:` principal; the verdict and accept paths run as the two
// actors above. Every other door stamps something else and is left alone. Two
// connector doors are not capture: the vCard ingest copies a curated card, and
// an import names its system in source_system.
//
// A human audit update naming full_name means somebody chose the name since.
// Only an update counts, because a capture's own create can read 'human' too.
//
// pairStale adds the repair's test: both split columns set, and a display that
// does not already contain both. A display holding the pair plus a nickname,
// title or affiliation says more than the pair, whoever wrote it.
func captureGuessedDisplayNamesTx(
	ctx context.Context, tx pgx.Tx, only *ids.ContactID, pairStale bool, limit int,
) ([]ids.ContactID, error) {
	rows, err := tx.Query(ctx, `
		SELECT p.id
		  FROM contact p
		 WHERE ($1::uuid IS NULL OR p.id = $1)
		   AND (p.captured_by LIKE 'connector:%' OR p.captured_by IN ($2, $3))
		   AND p.source IS DISTINCT FROM $4
		   AND p.source_system IS NULL
		   AND (NOT $5 OR (
		        p.first_name IS NOT NULL AND p.last_name IS NOT NULL
		        AND NOT (strpos(lower(p.full_name), lower(p.first_name)) > 0
		                 AND strpos(lower(p.full_name), lower(p.last_name)) > 0)))
		   AND NOT EXISTS (
		       SELECT 1 FROM audit_log a
		        WHERE a.entity_type = 'contact' AND a.entity_id = p.id
		          AND a.actor_type = 'human' AND a.action = 'update'
		          AND (a.after ? 'full_name' OR a.before ? 'full_name'))
		 ORDER BY p.id
		 LIMIT $6`,
		only, CaptureVerdictActor, CaptureAcceptActor, vcardSource, pairStale, limit)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading whose display name a contact carries: %w", err)
	}
	found, err := pgx.CollectRows(rows, pgx.RowTo[ids.ContactID])
	if err != nil {
		return nil, fmt.Errorf("contacts: reading whose display name a contact carries: %w", err)
	}
	return found, nil
}

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

	"github.com/margince/margince/backend/internal/platform/captureorigin"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The machine principals that mint a contact through the capture door after a
// sender has been judged. compose stamps its verdict and accept writes with
// these, so the origin test below and the writers cannot spell them apart.
const (
	CaptureVerdictActor = captureorigin.VerdictActor
	CaptureAcceptActor  = captureorigin.AcceptActor
)

// DisplayNameRepairActor is the principal the nightly participant_backfill job
// binds. Its display-name repairs are capture's own writes, so they never
// protect a name the way an explicit edit does.
const DisplayNameRepairActor = "system:participant_backfill"

// InitialsRepairTaskID names the initials data task. A data task names no
// migration, so its number is a version no namespace carries.
const InitialsRepairTaskID = "core/1792896243_initials_display_names"

// InitialsRepairActor is the principal the initials data task's batches bind.
// Its rewrite of capture's guess is still capture's, so a name learned later
// may replace it.
var InitialsRepairActor = datatask.ActorID(InitialsRepairTaskID)

// StaleDisplayNamesTx answers up to limit contacts whose display name the
// repair may replace with the pair their split columns carry. The repair's
// selector reads it, and RefreshDisplayNameTx asks the same statement about one
// contact. So a row the selector offers is one the writer accepts.
func StaleDisplayNamesTx(ctx context.Context, tx pgx.Tx, limit int) ([]ids.ContactID, error) {
	return captureGuessedDisplayNamesTx(ctx, tx, nil, nil, repairRule, limit)
}

// displayNameIsStaleTx is StaleDisplayNamesTx asked about one contact.
func displayNameIsStaleTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID) (bool, error) {
	found, err := captureGuessedDisplayNamesTx(ctx, tx, &contactID, nil, repairRule, 1)
	return len(found) == 1, err
}

// displayNameFollowsSignatureTx answers whether a name read from a signature may
// replace this contact's display name. A display that holds both halves is still
// replaced, because "Mai Lê" holds both in the opposite order from the
// signature's "Lê Thị Mai".
func displayNameFollowsSignatureTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID) (bool, error) {
	found, err := captureGuessedDisplayNamesTx(ctx, tx, &contactID, nil, signatureRule, 1)
	return len(found) == 1, err
}

// displayNameIsCapturesGuessTx answers whether the name fill may replace this
// contact's display name with the pair it is about to write. The split columns
// are still empty then, so the statement compares the display with that pair.
func displayNameIsCapturesGuessTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, pair ParsedName) (bool, error) {
	found, err := captureGuessedDisplayNamesTx(ctx, tx, &contactID, &pair, fillRule, 1)
	return len(found) == 1, err
}

// displayRule says which question captureGuessedDisplayNamesTx answers.
// needPair is the repair's rule that it never writes one half.
// keepWhenPairShown keeps a display that already holds both halves.
// unsplitMaxLen, when above zero, offers only contacts with no split columns and
// a display no longer than that.
// among, when set, asks only about those contacts.
type displayRule struct {
	needPair, keepWhenPairShown bool
	unsplitMaxLen               int
	among                       []ids.UUID
}

var (
	repairRule    = displayRule{needPair: true, keepWhenPairShown: true}
	fillRule      = displayRule{keepWhenPairShown: true}
	signatureRule = displayRule{needPair: true}
)

// captureGuessedDisplayNamesTx is the one statement that says whether a
// contact's display name may be replaced, for one contact (only) or for a page.
//
// Origin is captureorigin.ContactSQL, which the mailbox purge reads too.
//
// Any later audit row naming full_name protects the name, unless capture, the
// repair or the initials task wrote it. A human or an agent chose it, by update or by restore.
//
// The pair is the split columns, or the one the fill is about to write. A
// display containing both nonblank halves says at least as much as the pair,
// whoever wrote it, unless the rule says a signature outranks it. The enrich
// pass counts as a writer of full_name beside capture, as it does for the split
// columns. A later signature that disagrees is a better reading.
func captureGuessedDisplayNamesTx(
	ctx context.Context, tx pgx.Tx, only *ids.ContactID, pair *ParsedName, rule displayRule, limit int,
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
		   AND `+captureorigin.ContactSQL("$2", "$3", "$4")+`
		   AND ($12 = 0 OR (p.first_name IS NULL AND p.last_name IS NULL
		                    AND char_length(p.full_name) <= $12))
		   AND ($13::uuid[] IS NULL OR p.id = ANY($13))
		   AND (NOT $5 OR (n.first <> '' AND n.last <> ''))
		   AND NOT ($10 AND n.first <> '' AND n.last <> ''
		            AND strpos(lower(p.full_name), lower(n.first)) > 0
		            AND strpos(lower(p.full_name), lower(n.last)) > 0)
		   AND NOT EXISTS (
		       SELECT 1 FROM audit_log a
		        WHERE a.entity_type = 'contact' AND a.entity_id = p.id
		          AND a.action <> 'create'
		          AND (a.after ? 'full_name' OR a.before ? 'full_name')
		          AND NOT (`+captureWriterSQL("a.actor_id", "$2", "$3", "$9")+` OR a.actor_id IN ($11, $14)))
		 ORDER BY p.id
		 LIMIT $6`,
		only, CaptureVerdictActor, CaptureAcceptActor, vcardSource, rule.needPair, limit,
		first, last, DisplayNameRepairActor, rule.keepWhenPairShown, enrichCapturedBy,
		rule.unsplitMaxLen, rule.among, InitialsRepairActor)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading whose display name a contact carries: %w", err)
	}
	found, err := pgx.CollectRows(rows, pgx.RowTo[ids.ContactID])
	if err != nil {
		return nil, fmt.Errorf("contacts: reading whose display name a contact carries: %w", err)
	}
	return found, nil
}

// splitNameIsCapturesGuessTx reports whether a contact's split name columns are
// still only ever what a machine guessed.
//
// A display-name split is a guess. "Greven Ebusiness" parses to first `Greven`
// and last `Ebusiness`, and every greeting then uses it. What the conversation
// shows may replace that. What a colleague typed may not be replaced by either.
//
// Who wrote the column is the question, and the audit trail answers it. Any
// audit row naming first_name or last_name, written by somebody other than
// capture's own writers, protects the pair. Which actors count as capture's own
// comes from captureWriterSQL, which the display-name test above renders too.
//
// The enrich writer is admitted beside them, and that is this statement's own
// addition rather than captureWriterSQL's. A name the pass landed from one
// signature is still a machine's reading, so a later signature that disagrees
// is a better one. Were it left out, the first enrich write would protect the
// name against every later one.
func splitNameIsCapturesGuessTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID) (bool, error) {
	var guessed bool
	err := tx.QueryRow(ctx, `
		SELECT NOT EXISTS (
		       SELECT 1 FROM audit_log a
		        WHERE a.entity_type = 'contact' AND a.entity_id = $1
		          AND a.action <> 'create'
		          AND (a.after ?| $5::text[] OR a.before ?| $5::text[])
		          AND NOT (`+captureWriterSQL("a.actor_id", "$2", "$3", "$4")+`
		                   OR a.actor_id = $6))`,
		contactID, CaptureVerdictActor, CaptureAcceptActor, DisplayNameRepairActor,
		[]string{columnFirstName, columnLastName}, enrichCapturedBy).Scan(&guessed)
	if err != nil {
		return false, fmt.Errorf("contacts: reading whose name contact %s carries: %w", contactID, err)
	}
	return guessed, nil
}

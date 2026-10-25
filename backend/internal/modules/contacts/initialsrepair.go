// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Showing an initials-only address as its local part on contacts capture named
// before the parser kept "dv@" as "dv". A guess from the address and a folded
// header "DV" both store "Dv". Only the headers of the mails capture read as it
// created the contact can tell them apart. A name stays unless every readable
// one of them gave the address no name.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// InitialsNameCandidate is a contact whose display may be capture's guess from
// an initials-only address, with the mails that can show where it came from.
type InitialsNameCandidate struct {
	ContactID ids.ContactID
	// Address is the counterparty of every mail in Evidence, still live on
	// the contact.
	Address  string
	Evidence []InitialsNameEvidence
}

// InitialsNameEvidence is a stored original of a mail linked to the contact in
// the transaction that created it.
type InitialsNameEvidence struct {
	RawCaptureID ids.UUID
	// Connector names the original's format, read from the activity's
	// captured_by.
	Connector string
	// Outbound says the address is in To rather than From.
	Outbound bool
}

// maxInitialsEvidence bounds the originals one candidate costs a batch. A
// contact created with more linked mails than this keeps its name.
const maxInitialsEvidence = 20

// InitialsNamePage is one page of the walk: every contact it inspected, and
// the candidates among them.
type InitialsNamePage struct {
	Scanned    []ids.UUID
	Candidates []InitialsNameCandidate
}

// InitialsNamePageTx inspects up to limit live contacts with an address, after
// the given id, and answers which may show capture's initials guess. Its three
// statements read only those contacts, however few need a repair.
func InitialsNamePageTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (InitialsNamePage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return InitialsNamePage{}, err
	}
	if err := auth.Require(ctx, entityContact, principal.ActionRead); err != nil {
		return InitialsNamePage{}, err
	}
	rows, err := tx.Query(ctx, `
		SELECT c.id FROM contact c
		 WHERE c.archived_at IS NULL AND c.id > $1
		   AND EXISTS (SELECT 1 FROM contact_email e WHERE e.contact_id = c.id AND e.archived_at IS NULL)
		 ORDER BY c.id
		 LIMIT $2`, after, limit)
	if err != nil {
		return InitialsNamePage{}, fmt.Errorf("contacts: paging contacts for the initials repair: %w", err)
	}
	scanned, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil || len(scanned) == 0 {
		return InitialsNamePage{Scanned: scanned}, err
	}
	rule := displayRule{unsplitMaxLen: maxInitialsDisplayLen, among: scanned}
	guessed, err := captureGuessedDisplayNamesTx(ctx, tx, nil, nil, rule, len(scanned))
	if err != nil || len(guessed) == 0 {
		return InitialsNamePage{Scanned: scanned}, err
	}
	candidates, err := mailsLinkedAtCreateTx(ctx, tx, guessed)
	return InitialsNamePage{Scanned: scanned, Candidates: candidates}, err
}

// mailsLinkedAtCreateTx answers, for each contact, the mails linked to it in
// its own create transaction. Capture's ensure links the message it read. The
// verdict's cohort promotion links the rest. Those links carry the
// contact's created_at. Only mails whose counterparty is a live address of the
// contact count, and only ones whose original is still stored. A contact whose
// mails name two addresses, or more than maxInitialsEvidence mails, is left out.
func mailsLinkedAtCreateTx(ctx context.Context, tx pgx.Tx, contactIDs []ids.ContactID) ([]InitialsNameCandidate, error) {
	keys := make([]ids.UUID, 0, len(contactIDs))
	for _, id := range contactIDs {
		keys = append(keys, id.UUID)
	}
	rows, err := tx.Query(ctx, `
		SELECT p.id, m.address, m.raw_id, m.connector, m.outbound
		  FROM contact p
		  JOIN LATERAL (
		       SELECT lower(a.counterparty_email) AS address, rc.id AS raw_id,
		              split_part(a.captured_by, ':', 2) AS connector,
		              a.direction = 'outbound' AS outbound
		         FROM activity_link l
		         JOIN activity a ON a.id = l.activity_id AND a.kind = 'email'
		         JOIN raw_capture rc
		           ON rc.id = a.raw_capture_id
		           OR (a.raw_capture_id IS NULL
		               AND rc.source_system = a.source_system AND rc.source_id = a.source_id)
		        WHERE l.entity_type = 'contact' AND l.contact_id = p.id AND l.created_at = p.created_at
		          AND EXISTS (
		              SELECT 1 FROM contact_email e
		               WHERE e.contact_id = p.id AND e.archived_at IS NULL
		                 AND lower(e.email) = lower(a.counterparty_email))
		        ORDER BY rc.id
		        LIMIT $2) m ON true
		 WHERE p.id = ANY($1::uuid[])
		 ORDER BY p.id, m.raw_id`, keys, maxInitialsEvidence+1)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading the mails linked as a contact was created: %w", err)
	}
	var candidates []InitialsNameCandidate
	var current *InitialsNameCandidate
	keep := func() {
		if current != nil && len(current.Evidence) <= maxInitialsEvidence && current.Address != "" {
			candidates = append(candidates, *current)
		}
	}
	for rows.Next() {
		var id ids.ContactID
		var address string
		var evidence InitialsNameEvidence
		if err := rows.Scan(&id, &address, &evidence.RawCaptureID, &evidence.Connector, &evidence.Outbound); err != nil {
			rows.Close()
			return nil, fmt.Errorf("contacts: reading the mails linked as a contact was created: %w", err)
		}
		if current == nil || current.ContactID != id {
			keep()
			current = &InitialsNameCandidate{ContactID: id, Address: address}
		}
		if address != current.Address {
			current.Address = ""
		}
		current.Evidence = append(current.Evidence, evidence)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("contacts: reading the mails linked as a contact was created: %w", err)
	}
	keep()
	return candidates, nil
}

// RepairInitialsDisplayNameTx shows the local part of the candidate's address
// in place of capture's guess. headerNames are the display names its readable
// originals gave that address, and every one must name nobody. It answers
// whether it wrote. Every check is asked again under the row lock, so a name
// typed since the page was read stays.
func RepairInitialsDisplayNameTx(
	ctx context.Context, tx pgx.Tx, candidate InitialsNameCandidate, headerNames []string,
) (bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return false, err
	}
	if err := auth.Require(ctx, entityContact, principal.ActionUpdate); err != nil {
		return false, err
	}
	var previous string
	var split bool
	err := tx.QueryRow(ctx, `
		SELECT full_name, first_name IS NOT NULL OR last_name IS NOT NULL
		  FROM contact WHERE id = $1 FOR UPDATE`, candidate.ContactID).Scan(&previous, &split)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("contacts: reading the name contact %s shows: %w", candidate.ContactID, err)
	}
	if split || utf8.RuneCountInString(previous) > maxInitialsDisplayLen {
		return false, nil
	}
	local, ok := initialsDisplayReplacement(previous, headerNames, candidate.Address)
	if !ok {
		return false, nil
	}
	rule := displayRule{unsplitMaxLen: maxInitialsDisplayLen}
	guessed, err := captureGuessedDisplayNamesTx(ctx, tx, &candidate.ContactID, nil, rule, 1)
	if err != nil || len(guessed) != 1 {
		return false, err
	}
	live, err := mailsLinkedAtCreateTx(ctx, tx, guessed)
	if err != nil || len(live) != 1 || live[0].Address != candidate.Address ||
		!slices.Equal(live[0].Evidence, candidate.Evidence) {
		return false, err
	}
	return rewriteFullNameTx(ctx, tx, candidate.ContactID, previous, local)
}

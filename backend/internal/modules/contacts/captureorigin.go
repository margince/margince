// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Which contacts capture made and nobody has worked on since. The display-name
// repair and the mailbox purge both ask it, and both may only touch a record
// that is still capture's guess.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// capturedOriginSQL is the test that capture created the contact aliased p,
// read from captured_by, which is stamped once at create.
//
// Capture sinks run as `connector:` principals, the verdict and accept paths as
// their two actors. Two connector doors are not capture: the vCard ingest and an
// import, which names its system in source_system.
func capturedOriginSQL(verdict, accept, vcard string) string {
	return `(p.captured_by LIKE 'connector:%' OR p.captured_by IN (` + verdict + `, ` + accept + `))
		   AND p.source IS DISTINCT FROM ` + vcard + `
		   AND p.source_system IS NULL`
}

// captureWriterSQL is the test that capture or the display-name repair is the
// principal in column. A human, an agent, an import, an API caller or a missing
// stamp fails it.
func captureWriterSQL(column, verdict, accept, repair string) string {
	return `(coalesce(` + column + `, '') LIKE 'connector:%'
		OR coalesce(` + column + `, '') IN (` + verdict + `, ` + accept + `, ` + repair + `))`
}

// writtenAboutByAnybodySQL is the test that somebody other than capture wrote an
// activity linked to contact p, such as a note or a logged call.
// ColleagueWorkedOnSince does not read activities.
func writtenAboutByAnybodySQL(verdict, accept, repair string) string {
	return `EXISTS (
		       SELECT 1 FROM activity_link al
		         JOIN activity act ON act.id = al.activity_id
		        WHERE al.entity_type = 'contact' AND al.contact_id = p.id
		          AND NOT ` + captureWriterSQL("act.captured_by", verdict, accept, repair) + `)`
}

// CaptureMintedForSeatTx keeps the candidates a mailbox purge may anonymise for
// one seat: contacts capture created for that seat that nobody has worked on.
//
// The seat is the owner capture gave the record. The stamp names only the
// connector, so it cannot tell one seat's Gmail from another's.
//
// A later write by anybody but capture keeps the record: an edit, an owner
// change, a restore. A merge into it keeps it whoever ran it, and so does a lead
// promoted into it or made from it. Both may fold in a record somebody typed.
func CaptureMintedForSeatTx(ctx context.Context, tx pgx.Tx, seat ids.UUID, candidates []ids.UUID) ([]ids.UUID, error) {
	if seat == ids.Nil || len(candidates) == 0 {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT p.id
		  FROM contact p
		 WHERE p.id = ANY($1) AND p.owner_id = $2
		   AND `+capturedOriginSQL("$3", "$4", "$5")+`
		   AND NOT EXISTS (
		       SELECT 1 FROM lead l
		        WHERE l.promoted_contact_id = p.id OR l.from_contact_id = p.id)
		   AND NOT EXISTS (
		       SELECT 1 FROM audit_log a
		        WHERE a.entity_type = 'contact' AND a.entity_id = p.id
		          AND a.action <> 'create'
		          AND NOT `+captureWriterSQL("a.actor_id", "$3", "$4", "$6")+`)
		   -- A merge is audited on the contact it retired, naming the survivor.
		   AND NOT EXISTS (
		       SELECT 1 FROM audit_log m
		        WHERE m.entity_type = 'contact' AND m.action = 'merge'
		          AND m.after ->> $7 = p.id::text)
		   AND NOT `+writtenAboutByAnybodySQL("$3", "$4", "$6")+`
		 ORDER BY p.id`,
		candidates, seat, CaptureVerdictActor, CaptureAcceptActor, vcardSource, DisplayNameRepairActor,
		auditKeyMergedInto)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading which contacts capture made for a seat: %w", err)
	}
	minted, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("contacts: reading which contacts capture made for a seat: %w", err)
	}
	// A colleague's tag, list membership, relationship or merge, ever.
	kept := minted[:0]
	for _, id := range minted {
		worked, err := ColleagueWorkedOnSince(ctx, tx, "contact", id, time.Time{}, ids.Nil)
		if err != nil {
			return nil, err
		}
		if !worked {
			kept = append(kept, id)
		}
	}
	return kept, nil
}

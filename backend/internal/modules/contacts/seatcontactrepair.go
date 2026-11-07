// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type mintedContact struct {
	id      ids.ContactID
	version int64
}

// ArchiveSeatContactsTx archives the contacts on this page that capture minted
// for a live seat's login address. A colleague is a seat, never a contact.
//
// Minted means capture wrote it, and only the system or a connector acted on
// it or its edges since. That is the rule the edge repair beside it reads. A colleague's
// tag, list or link on it also keeps it: archiving would retire that work. The
// check runs under the contact's employment lock, and the archive pins the
// version read here, so a contact somebody edits meanwhile stays.
func ArchiveSeatContactsTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (SeatEmploymentPage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return SeatEmploymentPage{}, err
	}
	if err := auth.Require(ctx, entityContact, principal.ActionDelete); err != nil {
		return SeatEmploymentPage{}, err
	}
	page, minted, err := mintedContactPage(ctx, tx, after, limit)
	if err != nil {
		return SeatEmploymentPage{}, err
	}
	candidates := make([]ids.ContactID, len(minted))
	for i, contact := range minted {
		candidates[i] = contact.id
	}
	seats, err := seatContactsMatchingTx(ctx, tx, candidates, func(seats storekit.SeatAddresses, address string) bool {
		return seats.IsLogin(address)
	})
	if err != nil {
		return SeatEmploymentPage{}, err
	}
	for _, contact := range minted {
		if !seats[contact.id] {
			continue
		}
		if err := storekit.LockWriteIdentity(ctx, tx, employmentKind, contact.id.String()); err != nil {
			return SeatEmploymentPage{}, err
		}
		worked, err := workedOnUnderLock(ctx, tx, contact.id)
		if err != nil {
			return SeatEmploymentPage{}, err
		}
		if worked {
			continue
		}
		err = archiveContactInTx(ctx, tx, contact.id, &contact.version, writeOptions{})
		if errors.Is(err, apperrors.ErrVersionSkew) || errors.Is(err, apperrors.ErrNotFound) {
			continue
		}
		if err != nil {
			return SeatEmploymentPage{}, fmt.Errorf("contacts: archiving a seat's minted contact: %w", err)
		}
		page.Retired++
	}
	return page, nil
}

// untouchedByAnyoneSQL is the rule both reads below ask of contact p. No audit
// row on it or on an edge it is either end of is by any actor but the system or
// a connector. No human or agent tagged it or listed it. No lead names it, and
// nobody but capture wrote an activity about it.
func untouchedByAnyoneSQL() string {
	quoted := func(s string) string { return "'" + s + "'" }
	return `NOT EXISTS (SELECT 1 FROM audit_log a
		                WHERE a.entity_type = '` + contactEntity + `' AND a.entity_id = p.id
		                  AND a.actor_type NOT IN ('system', 'connector'))
		       AND NOT EXISTS (SELECT 1 FROM relationship r
		                JOIN audit_log a ON a.entity_type = '` + tableRelationship + `' AND a.entity_id = r.id
		                WHERE p.id IN (r.contact_id, r.counterparty_contact_id)
		                  AND a.actor_type NOT IN ('system', 'connector'))
		       AND NOT EXISTS (SELECT 1 FROM taggable g
		                WHERE g.entity_type = '` + contactEntity + `' AND g.entity_id = p.id
		                  AND g.assigned_by_kind IN ('human', 'agent'))
		       AND NOT EXISTS (SELECT 1 FROM list_member m
		                WHERE m.entity_type = '` + contactEntity + `' AND m.entity_id = p.id
		                  AND (m.added_by LIKE 'human:%' OR m.added_by LIKE 'agent:%'))
		       AND NOT ` + linkedFromALeadSQL + `
		       AND NOT ` + writtenAboutByAnybodySQL(quoted(CaptureVerdictActor), quoted(CaptureAcceptActor), quoted(DisplayNameRepairActor))
}

// workedOnUnderLock asks again, under the contact's employment lock, whether
// anyone acted on it since the page was read. It locks the contact row, so a
// tag or a list entry racing the archive either shows here or waits for it.
func workedOnUnderLock(ctx context.Context, tx pgx.Tx, contact ids.ContactID) (bool, error) {
	var untouched bool
	if err := tx.QueryRow(ctx, `SELECT `+untouchedByAnyoneSQL()+` FROM contact p WHERE p.id = $1 FOR UPDATE OF p`,
		contact).Scan(&untouched); err != nil {
		return false, fmt.Errorf("contacts: re-reading whether anyone acted on a seat's contact: %w", err)
	}
	if !untouched {
		return true, nil
	}
	return ColleagueWorkedOnSince(ctx, tx, contactEntity, contact.UUID, time.Time{}, ids.Nil)
}

// mintedContactPage reads one page of live contacts and the ones on it capture
// minted and nobody else touched.
func mintedContactPage(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (SeatEmploymentPage, []mintedContact, error) {
	rows, err := tx.Query(ctx, `
		SELECT p.id, p.version,
		       p.captured_by LIKE 'connector:%'
		       AND `+untouchedByAnyoneSQL()+`
		  FROM contact p
		 WHERE p.archived_at IS NULL AND p.merged_into_id IS NULL AND p.id > $1
		 ORDER BY p.id
		 LIMIT $2`, after, limit)
	if err != nil {
		return SeatEmploymentPage{}, nil, fmt.Errorf("contacts: paging contacts for the seat repair: %w", err)
	}
	defer rows.Close()
	var page SeatEmploymentPage
	var minted []mintedContact
	for rows.Next() {
		var contact mintedContact
		var untouched bool
		if err := rows.Scan(&contact.id, &contact.version, &untouched); err != nil {
			return SeatEmploymentPage{}, nil, fmt.Errorf("contacts: reading contacts for the seat repair: %w", err)
		}
		page.Scanned = append(page.Scanned, contact.id.UUID)
		if untouched {
			minted = append(minted, contact)
		}
	}
	if err := rows.Err(); err != nil {
		return SeatEmploymentPage{}, nil, fmt.Errorf("contacts: paging contacts for the seat repair: %w", err)
	}
	return page, minted, nil
}

// CountLiveContacts counts, up to limit+1, the contacts ArchiveSeatContactsTx
// pages.
func CountLiveContacts(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, limit int64,
) (int64, error) {
	if err := auth.Require(ctx, entityContact, principal.ActionRead); err != nil {
		return 0, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (
		SELECT 1 FROM contact WHERE archived_at IS NULL AND merged_into_id IS NULL LIMIT $1) live`,
		limit+1).Scan(&count)
	return count, err
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// The one answer to "is processing of this contact restricted" (GDPR Art. 18).
// A restriction is a live processing_restriction stop in communication_suppression;
// consent writes it, and every reader outside consent asks through here.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// RestrictedContacts answers which of contacts are restricted.
func RestrictedContacts(ctx context.Context, tx pgx.Tx, contacts []ids.UUID) (map[ids.UUID]bool, error) {
	return restrictedSet(ctx, tx, contacts, `
		SELECT DISTINCT rs.contact_id FROM communication_suppression rs
		 WHERE `+auth.LiveRestrictionSQL("rs")+` AND rs.contact_id = ANY($1)`)
}

// SubjectsRestricted answers whether any contact or lead a model request's
// subjects name is restricted. It drives from the live restrictions, which are few, and
// asks of each whether the subjects reach its contact.
func SubjectsRestricted(ctx context.Context, tx pgx.Tx, s model.Subjects) (bool, error) {
	if s.Empty() {
		return false, nil
	}
	var restricted bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		  SELECT 1 FROM communication_suppression rs
		   WHERE `+auth.LiveRestrictionSQL("rs")+` AND (rs.lead_id = ANY($8::uuid[]) OR rs.contact_id IS NOT NULL AND (
		         rs.contact_id = ANY($1::uuid[])
		      OR EXISTS (SELECT 1 FROM contact_email ce WHERE ce.contact_id = rs.contact_id
		                    AND lower(ce.email) = ANY(SELECT lower(a) FROM unnest($2::text[]) AS a))
		      OR EXISTS (SELECT 1 FROM activity_participant p WHERE p.activity_id = ANY($3::uuid[])
		                    AND (p.contact_id = rs.contact_id OR lower(p.address) IN (
		                         SELECT lower(ce.email) FROM contact_email ce WHERE ce.contact_id = rs.contact_id)))
		      OR EXISTS (SELECT 1 FROM activity_link l WHERE l.activity_id = ANY($3::uuid[])
		                    AND l.contact_id = rs.contact_id)
		      OR EXISTS (SELECT 1 FROM relationship r WHERE r.contact_id = rs.contact_id
		                    AND (r.company_id = ANY($4::uuid[]) OR r.deal_id = ANY($5::uuid[]) OR r.project_id = ANY($6::uuid[])))
		      OR EXISTS (SELECT 1 FROM deal_room_participant dp JOIN contact_email ce ON lower(ce.email) = dp.email
		                  WHERE dp.room_id = ANY($7::uuid[]) AND ce.contact_id = rs.contact_id))))`,
		s.Contacts, s.Addresses, s.Activities, s.Companies, s.Deals, s.Projects, s.Rooms, s.Leads).Scan(&restricted)
	if err != nil {
		return false, fmt.Errorf("storekit: reading restrictions of processing: %w", err)
	}
	return restricted, nil
}

func restrictedSet(ctx context.Context, tx pgx.Tx, subjects []ids.UUID, query string) (map[ids.UUID]bool, error) {
	out := map[ids.UUID]bool{}
	if len(subjects) == 0 {
		return out, nil
	}
	rows, err := tx.Query(ctx, query, subjects)
	if err != nil {
		return nil, fmt.Errorf("storekit: reading restrictions of processing: %w", err)
	}
	found, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("storekit: reading restrictions of processing: %w", err)
	}
	for _, id := range found {
		out[id] = true
	}
	return out, nil
}

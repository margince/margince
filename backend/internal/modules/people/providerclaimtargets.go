// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Finding what a bought value attaches to, and attaching it.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// insertSocialHandle puts one platform handle on a contact, leaving any handle
// already there alone, and reports whether it landed.
//
// Shared with the LinkedIn-match apply, which had this statement first: a
// handle on the record is somebody's statement, and neither confirming a match
// nor buying a profile is grounds to replace it. Two writers of one rule, one
// spelling.
//
// It takes NO lock. Every caller holds the subject from the top of its own
// transaction — ApplyLinkedInMatch through HoldWritableLive, the provider
// hand-off through the holding fence — and re-taking it here would be the
// ordering the eraser deadlocks against.
func insertSocialHandle(ctx context.Context, tx pgx.Tx, personID ids.UUID, platform, handle string) (bool, error) {
	tag, err := tx.Exec(ctx, `
		INSERT INTO person_social (person_id, platform, handle)
		VALUES ($1, $3, $2)
		ON CONFLICT (person_id, platform) DO NOTHING`, personID, handle, platform)
	if err != nil {
		return false, fmt.Errorf("people: writing a contact's %s handle: %w", platform, err)
	}
	return tag.RowsAffected() > 0, nil
}

// organizationByDomain answers which company owns a domain, and whether one
// does at all.
//
// Domain only, never display name. Two live companies may share a name — the
// schema permits it and nothing dedupes it — so matching on one would attach a
// contact's employment to whichever row sorted first, which is a false
// statement about where somebody works. A domain is unique by constraint.
func organizationByDomain(ctx context.Context, tx pgx.Tx, domain string) (ids.OrganizationID, bool, error) {
	var org ids.OrganizationID
	err := tx.QueryRow(ctx, `
		SELECT d.organization_id
		  FROM organization_domain d
		  JOIN organization o ON o.id = d.organization_id
		 WHERE lower(d.domain) = lower($1)
		   AND d.archived_at IS NULL
		   AND o.archived_at IS NULL
		 LIMIT 1`, domain).Scan(&org)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.OrganizationID{}, false, nil
	}
	if err != nil {
		return ids.OrganizationID{}, false, fmt.Errorf("people: resolving a bought employer's domain: %w", err)
	}
	return org, true, nil
}

// plantProviderEmploymentEdge attaches a contact to a company a provider named,
// and only when they hold no current employer.
//
// The same two partial uniques guard it as the capture path's edge, and
// ON CONFLICT DO NOTHING makes either a no-op: a purchase has nothing to add to
// an employment that already exists, and it never reassigns one.
//
// The audit says `origin: provider`, not `capture`. Somebody was paid to assert
// this, which is a different kind of claim from one inferred out of the
// installation's own correspondence.
func plantProviderEmploymentEdge(ctx context.Context, tx pgx.Tx, personID ids.UUID, orgID ids.OrganizationID, providerName string) (ids.UUID, bool, error) {
	subject := ids.PersonID{UUID: personID}
	// The edge hangs off the person, so an archive in flight must not be
	// outrun — the same reason the capture path's edge takes it. The hand-off
	// holds this subject already; re-taking the same lock in the same
	// transaction is free, and it is what makes this writer's own correctness
	// readable without tracing back to its caller.
	if err := lockPersonForAttach(ctx, tx, subject); err != nil {
		return ids.UUID{}, false, err
	}
	var edgeID ids.UUID
	err := tx.QueryRow(ctx, `
		INSERT INTO relationship (kind, person_id, organization_id, is_current_primary, source, captured_by)
		SELECT 'employment', $1, $2, true, $3, $4
		WHERE NOT EXISTS (
			SELECT 1 FROM relationship
			WHERE person_id = $1 AND `+CurrentPrimarySlotSQL("")+`)
		ON CONFLICT DO NOTHING
		RETURNING id`,
		personID, orgID, providerName, connectorCapturedBy(providerName)).Scan(&edgeID)
	if errors.Is(err, pgx.ErrNoRows) {
		// Either guard skipped it: the contact already has a current employer,
		// or this exact edge exists. Nothing written, so nothing audited.
		return ids.UUID{}, false, nil
	}
	if err != nil {
		return ids.UUID{}, false, fmt.Errorf("people: linking a contact to a bought employer: %w", err)
	}
	if err := auditCapturedEmployment(ctx, tx, edgeID, subject, orgID, relationshipOriginProvider); err != nil {
		return ids.UUID{}, false, err
	}
	return edgeID, true, nil
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// The lead identity probes (ADR-0008 keeps leads out of contact matching, so
// they carry exact keys of their own: a live email and a LinkedIn profile
// URL). Two write shapes create leads — the direct create and the
// evidence-backed capture — and they answer a hit with deliberately
// different policies: the direct create refuses with the incumbent's id,
// capture stages a merge proposal for a human. What they must NOT hold
// separately is the question itself, which is why the SQL lives here rather
// than once per module: the modules cannot import each other, and a second
// copy of "is this identity already claimed" is how the two drifted apart
// before (a LinkedIn URL was probed by neither).
//
// This is the same seam EmailSuppressed already occupies — a tx-carrying
// predicate that both contacts and capture call.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// LeadSourceKey names the source record a probe must not report against
// itself. Re-capturing one message is a replay, not a collision, and without
// this the second delivery of the same mail would stage a merge proposal
// pairing a lead with itself.
type LeadSourceKey struct {
	SourceSystem string
	SourceID     string
}

// LiveLeadByEmail reports the live lead holding this address, if any. The
// address is lowercased by the query to match lead_email_norm's storage
// contract, so a caller that has not normalized still compares like for like.
func LiveLeadByEmail(ctx context.Context, tx pgx.Tx, email string, exclude *LeadSourceKey) (ids.LeadID, bool, error) {
	if email == "" {
		return ids.LeadID{}, false, nil
	}
	return liveLeadBy(ctx, tx, leadEmail, email, exclude, nil)
}

// LiveLeadByLinkedInURL reports the live lead holding this profile URL. The
// caller normalizes first (contacts.NormalizeLinkedInURL) — a profile URL has
// no SQL-side normalization to fall back on, so an unnormalized argument
// finds nothing rather than matching loosely.
//
// The earliest row wins when a workspace already carries duplicates from
// before this probe existed: idx_lead_linkedin is deliberately NOT unique
// (merging what is already there is a human decision), so the probe has to
// pick the canonical original deterministically rather than assume one row.
func LiveLeadByLinkedInURL(ctx context.Context, tx pgx.Tx, normalizedURL string, exclude *LeadSourceKey) (ids.LeadID, bool, error) {
	if normalizedURL == "" {
		return ids.LeadID{}, false, nil
	}
	return liveLeadBy(ctx, tx, leadLinkedIn, normalizedURL, exclude, nil)
}

// leadKey is one exclusive key a probe asks about. The predicate is a fixed
// literal, never caller text; the value is always a bind parameter.
type leadKey struct{ predicate, lane string }

var (
	leadEmail    = leadKey{predicate: `email = lower($1)`, lane: "email"}
	leadLinkedIn = leadKey{predicate: `linkedin_url = $1`, lane: "linkedin_url"}
)

// LiveLeadEmailHeldOutside asks whether a live lead outside except holds the
// address. The address has one live holder at most (uq_lead_email_dedupe), but
// that one may be in except.
func LiveLeadEmailHeldOutside(ctx context.Context, tx pgx.Tx, email string, except []ids.UUID) (bool, error) {
	if email == "" {
		return false, nil
	}
	_, held, err := liveLeadBy(ctx, tx, leadEmail, email, nil, except)
	return held, err
}

// LiveLeadLinkedInHeldOutside asks the same of a normalized profile URL, which
// idx_lead_linkedin lets several live leads hold.
func LiveLeadLinkedInHeldOutside(ctx context.Context, tx pgx.Tx, normalizedURL string, except []ids.UUID) (bool, error) {
	if normalizedURL == "" {
		return false, nil
	}
	_, held, err := liveLeadBy(ctx, tx, leadLinkedIn, normalizedURL, nil, except)
	return held, err
}

// liveLeadBy is the one query shape behind every probe: the earliest live
// holder of key outside exclude's source record and the except ids.
func liveLeadBy(
	ctx context.Context, tx pgx.Tx, key leadKey, value string, exclude *LeadSourceKey, except []ids.UUID,
) (ids.LeadID, bool, error) {
	var excludeSystem, excludeID *string
	if exclude != nil {
		excludeSystem, excludeID = &exclude.SourceSystem, &exclude.SourceID
	}
	var id ids.LeadID
	err := tx.QueryRow(ctx, `
		SELECT id FROM lead
		 WHERE `+key.predicate+` AND archived_at IS NULL
		   AND ($2::text IS NULL
		        OR source_system IS DISTINCT FROM $2
		        OR source_id IS DISTINCT FROM $3)
		   AND ($4::uuid[] IS NULL OR id <> ALL($4))
		 ORDER BY created_at, id
		 LIMIT 1`, value, excludeSystem, excludeID, except).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.LeadID{}, false, nil
	}
	if err != nil {
		return ids.LeadID{}, false, fmt.Errorf("probe lead %s dedupe: %w", key.lane, err)
	}
	return id, true, nil
}

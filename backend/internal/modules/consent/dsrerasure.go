// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Fulfilling an erasure request, which is the one case whose closure runs
// another engine.
//
// Split from dsr.go, which owns the case as a record: opening it, moving it
// between statuses, reading it back. This is what happens when one particular
// closure has a side effect on the subject's whole footprint, and the two
// change for different reasons.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// FulfilErasure fulfils an erasure request atomically with respect to every
// other officer touching the same row. It locks the request FOR UPDATE and
// HOLDS that lock across the injected erase, so a concurrent UpdateDSR on this
// same request blocks on the lock (then loses the transition as illegal) rather
// than slipping a reject/fulfil in between the read that proved this fulfil
// legal and the scrub that acts on it — the race that would otherwise leave a
// subject erased on a request the queue still shows open or rejected.
//
// erase is the privacy engine's cross-store scrub (compose injects it via the
// Eraser seam); it commits in its OWN transaction — consent owns
// data_subject_request, privacy owns the contact/capture/retrieval erase, and no
// single transaction may legally span both. Ordering carries the guarantee: the
// scrub MUST land before the status flips to fulfilled. A finalize that fails
// after the scrub committed leaves an already-erased subject on a still-open
// request, which a retry re-fulfils harmlessly (EraseContact anonymizes in place
// and is idempotent) — never a request certified fulfilled over an erase that
// never ran. Because we hold the request lock (not the contact rows) while erase
// checks out a second pooled connection for its own transaction, the two never
// contend: this nests one connection deep, well within the pool on the
// human-driven, admin-only DSR surface.
func (s *Store) FulfilErasure(ctx context.Context, id ids.UUID, in UpdateDSRInput,
	erase func(ctx context.Context, contactID ids.UUID, reason string) error,
) (dsrRow, error) {
	if err := requireDSRAdmin(ctx, principal.ActionUpdate); err != nil {
		return dsrRow{}, err
	}
	// ids.Parse proves syntax only; a subject_ref that fails even that names
	// no contact at all. Both doors — unparseable, and syntactically valid but
	// naming nobody (the erase's ErrNotFound) — converge on this one refusal.
	unresolvedSubject := &ValidationError{
		Field:  fieldSubjectRef,
		Reason: "an erasure request must name a contact id before it can be fulfilled",
	}
	var out dsrRow
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		current, err := scanDSR(tx.QueryRow(ctx, dsrSelectForUpdate, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		if verr := validateDSRUpdate(current, in); verr != nil {
			return verr
		}
		// THE LINK FIRST, then the reference.
		//
		// An erasure retires every other case the subject holds, which
		// tombstones subject_ref on them — so a SECOND erasure case for one
		// contact would be unfulfillable if this read only the reference:
		// the first fulfilment would have made the second impossible to
		// resolve, leaving an open case nobody can close.
		//
		// The link survives that retirement on an open case for exactly this,
		// and the reference answers for the officer-created cases that never
		// carried a link.
		contactID, resolved := resolveDSRSubject(current)
		if !resolved {
			return unresolvedSubject
		}
		if err := erase(ctx, contactID, "dsr:"+current.ID.String()); err != nil {
			if errors.Is(err, apperrors.ErrNotFound) {
				return unresolvedSubject
			}
			return err
		}
		out, err = finalizeErasureFulfil(ctx, tx, id, in, current)
		return err
	})
	return out, err
}

// finalizeErasureFulfil flips the FOR UPDATE-locked request to fulfilled and
// appends the audit row, run inside the caller's held-lock transaction (never
// on its own). The AND status guard mirrors UpdateDSR's finalize as defense in
// depth — with the lock held it can only match, but a miss still maps to the
// honest illegal-transition error rather than a silent no-op.
func finalizeErasureFulfil(ctx context.Context, tx pgx.Tx, id ids.UUID, in UpdateDSRInput, current dsrRow) (dsrRow, error) {
	// THE RESOLUTION COMES OUT, and subject_ref deliberately does NOT.
	//
	// The erasure has just run and took the subject out of every other case
	// they opened. It could not take them out of THIS one: this transaction
	// holds the row FOR UPDATE across that call, so the engine's retirement
	// skips it rather than blocking on a lock its caller holds.
	//
	// The prose goes, because an officer typed it and it can name or quote the
	// subject. The reference stays, because THIS ROW is how a replayed
	// fulfilment finds the contact to erase — the status is already
	// 'fulfilled', which validateDSRUpdate treats as a legal no-op, and
	// tombstoning the reference makes that retry fail to resolve a subject it
	// is supposed to be idempotent about.
	//
	// SO THE ERASURE REQUEST KEEPS THE IDENTITY IT WAS MADE WITH, alone among
	// the subject's cases. That is a real gap and it is bounded: one row, one
	// column, carrying what the subject themselves wrote on a form to ask for
	// this. Closing it means making the replay resolve the contact some other
	// way — the audit trail, or a column the tombstone does not touch — which
	// is a change to how a fulfilment re-finds its subject rather than to what
	// this statement writes.
	row := tx.QueryRow(ctx, `
			UPDATE data_subject_request SET
			  status = 'fulfilled',
			  assignee_id = coalesce($2, assignee_id),
			  resolution = CASE
			    WHEN coalesce($3, resolution) IS NULL THEN NULL ELSE 'erased' END,
			  contact_id = NULL
			WHERE id = $1 AND status = $4
			RETURNING `+dsrColumns,
		id, in.AssigneeID, in.Resolution, current.Status)
	out, err := scanDSR(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return dsrRow{}, illegalTransition(current.Status, "fulfilled")
		}
		return dsrRow{}, err
	}
	if _, err := storekit.Audit(ctx, tx, "update", "data_subject_request", id, map[string]any{
		fieldStatus: current.Status,
	}, map[string]any{
		fieldStatus: out.Status, fieldResolution: in.Resolution != nil,
	}); err != nil {
		return dsrRow{}, err
	}
	return out, nil
}

// resolveDSRSubject answers which contact this case is about.
//
// TWO SOURCES, because neither is always set. A case opened through the
// subject's own confirm link carries contact_id; one an officer opened by hand
// carries only subject_ref, which CreateDSR takes as free text. And an erasure
// tombstones the reference on every case but the one it is fulfilling, so on a
// sibling case the link is the only thing left to read.
//
// Both come off the row this transaction already holds FOR UPDATE, so there is
// no second read to bound.
func resolveDSRSubject(current dsrRow) (ids.UUID, bool) {
	if current.ContactID != nil {
		return *current.ContactID, true
	}
	contactID, parseErr := ids.Parse(current.SubjectRef)
	if parseErr != nil {
		return ids.UUID{}, false
	}
	return contactID, true
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package integrations

// Whose scope a submission travels under.
//
// A run is queued by a contact and submitted by the CONNECTOR: actingForProvider
// replaces the actor with a system principal before anything leaves, and a
// system principal passes every object gate. So the question "may the employer
// travel with this subject" cannot be asked of the context at submission time —
// it would always answer yes — and has to be asked of the stored requester
// instead.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// RequesterHoldsFunc answers whether the human who asked for a run still holds
// the grant that lets an employer travel with the subject.
//
// A seam for the reason every other one in this package is: identity is a
// sibling module, and compose injects the edge.
type RequesterHoldsFunc func(ctx context.Context, tx pgx.Tx, userID string) (bool, error)

// WithRequesterStanding binds the question the submission asks about the human
// who queued a run, rather than about the connector executing it.
func (s *Store) WithRequesterStanding(holds RequesterHoldsFunc) *Store {
	s.requesterHolds = holds
	return s
}

// employerWithheldFrom reports whether the human who asked for this run may not
// read companies, so the employer must not travel with the subject.
//
// A run with no requester is the automatic sweep: it acts for the installation
// rather than for a contact, and there is no scope to answer for.
func (s *Store) employerWithheldFrom(ctx context.Context, tx pgx.Tx, runID string) (bool, error) {
	if s.requesterHolds == nil {
		return false, errors.New("integrations: no requester-standing check is bound, so a run cannot say whose scope it travels under")
	}
	var requester *string
	if err := tx.QueryRow(ctx,
		`SELECT requested_by::text FROM provider_run WHERE id = $1`, runID).Scan(&requester); err != nil {
		return false, fmt.Errorf("integrations: reading who asked for run %s: %w", runID, err)
	}
	if requester == nil {
		return false, nil
	}
	holds, err := s.requesterHolds(ctx, tx, *requester)
	if err != nil {
		return false, err
	}
	return !holds, nil
}

// withholdEmployerIfUnreadable strips the employer from a request when the
// human who asked for the run may not read companies, and reports whether
// what remains is still worth sending.
//
// Withheld rather than refused: an employer they may not read is one field of a
// subject who is otherwise perfectly matchable, and refusing the whole run
// would tell them the vendor found nothing when a permission stopped us. It is
// the same shape SubjectIdentifiers returns to a caller without the grant,
// restored here after the connector's own re-read undid it.
func (s *Store) withholdEmployerIfUnreadable(
	ctx context.Context, tx pgx.Tx, runID string, req *provider.Request, desc provider.Descriptor,
) (bool, error) {
	withheld, err := s.employerWithheldFrom(ctx, tx, runID)
	if err != nil || !withheld {
		return false, err
	}
	req.Identifiers.CompanyName, req.Identifiers.CompanyDomain = "", ""
	return !req.Identifiers.Matchable(desc.MatchRules), nil
}

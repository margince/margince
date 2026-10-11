// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// When a data-subject request must be answered, and which rule says so.
//
// The installation's selected jurisdiction decides. Its pack may declare a
// deadline per request kind. A kind it leaves out gets the GDPR month
// (Art. 12(3)), and so does an installation with no jurisdiction selected.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/subjectrightsrules"
)

// requestDeadlineFunc resolves one request's deadline inside the caller's
// transaction. The confirm-link writer is a package function, so the store's
// resolver travels to it as a value.
type requestDeadlineFunc func(ctx context.Context, tx pgx.Tx, kind string, received time.Time) (due time.Time, rule string, err error)

// requestDeadlineTx is the one resolver both request writers use. A failed
// country read is returned, never treated as "no jurisdiction": the GDPR month
// can be later than the selected pack's deadline.
func (s *Store) requestDeadlineTx(ctx context.Context, tx pgx.Tx, kind string, received time.Time) (time.Time, string, error) {
	code, err := s.installationCountry(ctx, tx)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("consent: reading the jurisdiction a request deadline comes from: %w", err)
	}
	deadline, rule := subjectrightsrules.Resolve(code, subjectrightsrules.RequestKind(kind))
	return deadline.Due(received), rule, nil
}

// RequestDeadline answers the deadline a request of this kind received at
// received would get, for the form that files one. Gated like the queue it
// fills: the deadline is shown only to whoever may file the request.
//
// received goes through receivedAtFor, so the preview refuses what filing
// refuses.
func (s *Store) RequestDeadline(ctx context.Context, kind string, stated *time.Time) (time.Time, string, error) {
	if err := requireDSRAdmin(ctx, principal.ActionRead); err != nil {
		return time.Time{}, "", err
	}
	if err := subjectrightsrules.RequestKind(kind).Validate(); err != nil {
		return time.Time{}, "", &ValidationError{Field: fieldKind, Reason: "not a request kind"}
	}
	received, err := receivedAtFor(stated, time.Now())
	if err != nil {
		return time.Time{}, "", err
	}
	var due time.Time
	var rule string
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		due, rule, err = s.requestDeadlineTx(ctx, tx, kind, received)
		return err
	})
	return due, rule, err
}

// receivedAtFor settles when a manually filed request arrived: now when the
// officer did not say, and never later than now. A request logged days late
// keeps the deadline its real arrival gives it; a future arrival would grant
// days the subject is not owed. A few minutes of client clock skew is read as
// now rather than refused.
func receivedAtFor(stated *time.Time, now time.Time) (time.Time, error) {
	if stated == nil {
		return now, nil
	}
	const skew = 5 * time.Minute
	switch {
	case stated.After(now.Add(skew)):
		return time.Time{}, &ValidationError{Field: fieldReceivedAt, Reason: "a request cannot have arrived in the future"}
	case stated.After(now):
		return now, nil
	}
	return *stated, nil
}

// dueAtFor settles the stored deadline against the computed one. An absent
// date takes the computed one. A later date is refused, because it would
// show a request as on time on a day it is already late. An earlier date is
// the officer's choice and is marked as such.
func dueAtFor(stated *time.Time, computed time.Time, rule string) (due time.Time, overridden bool, err error) {
	if stated == nil {
		return computed, false, nil
	}
	if stated.After(computed) {
		return time.Time{}, false, &ValidationError{Field: fieldDueAt, Reason: fmt.Sprintf(
			"later than the statutory deadline %s (%s); choose that date or an earlier one", computed.UTC().Format(time.RFC3339), rule)}
	}
	return *stated, !stated.Equal(computed), nil
}

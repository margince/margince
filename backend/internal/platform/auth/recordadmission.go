// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// The whole admission a single-record read or edit passes, as one call per
// verb. The row probes in rowscope.go and writescope.go answer only "which
// rows"; a record page and a "who can see this" answer both need the object
// grant and the seat ceiling as well, and asking the same composition from both
// is what keeps them agreeing.
//
// Each verb is written once, for a list of principals. The record's own read
// and edit paths ask it for their one caller; the "who can see this" read asks
// it for every member, a bounded number of principals per statement.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// admissionBatch bounds how many principals one statement judges, so a large
// installation costs several ordinary statements rather than one enormous one.
const admissionBatch = 100

// EnsureReadable admits a read of one record: the object grant on its type,
// then the row gate. An archived record still reads — the record page opens one
// through its archived filter — so the live filter is the caller's to add.
func EnsureReadable(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) error {
	return admitActor(ctx, tx, table, id, AdmitReaders)
}

// EnsureChangeable admits an edit of one record: the object's update grant,
// write authority over a LIVE row, then the seat ceiling.
//
// The ceiling is asked here although the session door already refuses a read
// seat's mutating request, because this is also the answer a "who can change
// this" read gives about somebody who is not making a request at all. It comes
// last so a read seat's standing write share is still refused by the write arm
// itself, and it compares against SeatRead rather than asking CanMutate for the
// reason writeAuthorityPredicateAs gives: an internal principal with no seat to
// resolve is not a read seat.
func EnsureChangeable(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) error {
	return admitActor(ctx, tx, table, id, AdmitChangers)
}

type admission func(ctx context.Context, tx pgx.Tx, table string, id ids.UUID, ps []principal.Principal) ([]error, error)

func admitActor(ctx context.Context, tx pgx.Tx, table string, id ids.UUID, admit admission) error {
	p, err := rbacActor(ctx)
	if err != nil {
		return err
	}
	refusals, err := admit(ctx, tx, table, id, []principal.Principal{p})
	if err != nil {
		return err
	}
	return refusals[0]
}

// verdict turns a statement's answers into one principal's refusal, or nil.
type verdict func(answers []bool) error

// AdmitReaders answers EnsureReadable for each principal: nil where the read is
// admitted, and the refusal EnsureReadable gives where it is not. The row gate
// is EnsureVisible's: the principal's scope clause over the row, and no query
// at all for a principal whose clause is empty.
func AdmitReaders(ctx context.Context, tx pgx.Tx, table string, id ids.UUID, ps []principal.Principal) ([]error, error) {
	if !ownerScopedTables[table] {
		return nil, fmt.Errorf("auth: %q is not a row-scoped table", table)
	}
	return admitInBatches(ctx, tx, id, ps, func(p principal.Principal, st *admissionStatement) (verdict, error) {
		pctx := principal.WithActor(ctx, p)
		if err := Require(pctx, table, principal.ActionRead); err != nil {
			return refused(err), nil
		}
		clause, err := ScopeClauseFor(pctx, table, "", st.arg)
		if err != nil || clause == "" {
			return admitted, err
		}
		at := st.probe(table, clause)
		return func(answers []bool) error { return notFoundUnless(answers[at]) }, nil
	})
}

// AdmitChangers answers EnsureChangeable for each principal. The row halves are
// EnsureWritableLive's: visible and live, then the write arm, which an
// unbounded principal or a table no share can name skips.
func AdmitChangers(ctx context.Context, tx pgx.Tx, table string, id ids.UUID, ps []principal.Principal) ([]error, error) {
	if !ownerScopedTables[table] {
		return nil, fmt.Errorf("auth: %q is not a row-scoped table", table)
	}
	return admitInBatches(ctx, tx, id, ps, func(p principal.Principal, st *admissionStatement) (verdict, error) {
		pctx := principal.WithActor(ctx, p)
		if err := Require(pctx, table, principal.ActionUpdate); err != nil {
			return refused(err), nil
		}
		clause, err := ScopeClauseFor(pctx, table, "", st.arg)
		if err != nil {
			return nil, err
		}
		live := "archived_at IS NULL"
		if clause != "" {
			live += " AND " + clause
		}
		visible := st.probe(table, live)
		write := -1
		if !Unbounded(p) && shareableTables[table] {
			write = st.probe(table, writeAuthorityPredicate(p, table, st.arg))
		}
		return func(answers []bool) error {
			if err := notFoundUnless(answers[visible]); err != nil {
				return err
			}
			if write >= 0 && !answers[write] {
				return apperrors.ErrPermissionDenied
			}
			if p.SeatType == principal.SeatRead {
				return fmt.Errorf("%s.%s: %w", table, principal.ActionUpdate, apperrors.ErrSeatTierInsufficient)
			}
			return nil
		}, nil
	})
}

// admissionStatement collects the row probes of up to admissionBatch
// principals into one SELECT of booleans, with its own parameter list.
type admissionStatement struct {
	args   []any
	idPos  int
	probes []string
}

func newAdmissionStatement(id ids.UUID) *admissionStatement {
	st := &admissionStatement{}
	st.idPos = st.arg(id)
	return st
}

//craft:ignore naked-any the argument registrar the scope clauses bind through takes a value of whatever type each clause registers
func (st *admissionStatement) arg(v any) int {
	st.args = append(st.args, v)
	return len(st.args)
}

// probe adds "does the row pass this predicate" and returns its index in the
// answer. table is one of the closed set the callers checked.
func (st *admissionStatement) probe(table, predicate string) int {
	st.probes = append(st.probes,
		fmt.Sprintf(`EXISTS (SELECT 1 FROM %s WHERE id = $%d AND %s)`, table, st.idPos, predicate))
	return len(st.probes) - 1
}

func (st *admissionStatement) run(ctx context.Context, tx pgx.Tx) ([]bool, error) {
	if len(st.probes) == 0 {
		return nil, nil
	}
	var answers []bool
	err := tx.QueryRow(ctx, "SELECT ARRAY["+strings.Join(st.probes, ", ")+"]::boolean[]", st.args...).Scan(&answers)
	return answers, err
}

// admitInBatches renders each principal's probes with plan, runs one statement
// per admissionBatch principals, and hands each principal its answers.
func admitInBatches(
	ctx context.Context, tx pgx.Tx, id ids.UUID, ps []principal.Principal,
	plan func(principal.Principal, *admissionStatement) (verdict, error),
) ([]error, error) {
	out := make([]error, len(ps))
	for start := 0; start < len(ps); start += admissionBatch {
		chunk := ps[start:min(start+admissionBatch, len(ps))]
		st := newAdmissionStatement(id)
		verdicts := make([]verdict, len(chunk))
		for i, p := range chunk {
			v, err := plan(p, st)
			if err != nil {
				return nil, err
			}
			verdicts[i] = v
		}
		answers, err := st.run(ctx, tx)
		if err != nil {
			return nil, err
		}
		for i, v := range verdicts {
			out[start+i] = v(answers)
		}
	}
	return out, nil
}

func admitted([]bool) error { return nil }

func refused(err error) verdict {
	return func([]bool) error { return err }
}

func notFoundUnless(ok bool) error {
	if ok {
		return nil
	}
	return apperrors.ErrNotFound
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// Row scope for a SIGNAL, which inherits from the record it is about.
//
// The activity half of the same idea is in inheritedscope.go, and the two are
// one policy: a record with no owner_id of its own borrows the sensitivity of
// what it points at. Its own file because a signal's rule has a second half an
// activity's does not — a signal ALSO carries its own visibility, because its
// evidence can be narrower than its subject.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SignalScopeClause is the signal analogue of ActivityDiscoverClause: a
// A signal is visible when its SUBJECT is visible and its own visibility
// admits the reader. A subject-less signal (a raw item still awaiting
// resolution) is workspace-shared, like an unlinked note.
//
// The subject arm is the older half: a signal's free-text summary and evidence
// inherit the sensitivity of the record it is ABOUT. That was the whole rule
// while a signal's evidence could only come from records at least as visible as
// its subject — the producers reached an account through a direct activity_link
// row, which is the same link that makes an activity readable to that account's
// readers (ActivityDiscoverClause is the any-link rule).
//
// The producers now also reach an account through the employer of the contact a
// message is filed against, and through its deal. Neither is a link on the
// activity, so a signal's evidence can be narrower than its subject, and the
// signal carries its own visibility to say so. It is capture privacy, so it
// does NOT yield to row_scope=all: an admin reading a colleague's unpromoted
// correspondence through a summary of it is the same disclosure the boundary
// exists to prevent, taking the long way round.
//
// It lives here, not in the signals module, because the signals store's reads
// and the approvals surface's staged-archive visibility probe both enforce it
// — scope policy has exactly one spelling (ADR-0054 §8). alias names the
// signal table in the outer query.
func SignalScopeClause(ctx context.Context, alias string, arg func(any) int) (string, error) {
	p, err := rbacActor(ctx)
	if err != nil {
		return "", err
	}
	// The system principal — the producers themselves, the relay, the privacy
	// engines — reads both arms away. Everyone else faces the private arm,
	// unbounded or not.
	if p.Type == principal.PrincipalSystem {
		return "", nil
	}
	private := fmt.Sprintf("(%[1]s.visibility <> 'owner' OR %[1]s.owner_id = $%d)",
		alias, arg(p.UserID))
	if UnboundedFor(p, tablePerson, tableCompany, tableDeal, tableProject) {
		return private, nil
	}
	person := VisiblePredicate(p, tablePerson, arg)
	company := VisiblePredicate(p, tableCompany, arg)
	deal := VisiblePredicate(p, tableDeal, arg)
	// A project-subject signal inherits the project's visibility, the same
	// way the three older subjects do; an arm missing here would DROP such a
	// signal for every bounded reader, not withhold it for a reason. The arm
	// also takes the project OBJECT grant, which the row predicate alone does
	// not ask: a project's row scope admits every seat, so without the grant
	// half a seat holding signal.read and no project.read would read a summary
	// that names a project it may not list.
	project := VisiblePredicate(p, tableProject, arg)
	projectArm := sqlNoRow
	if p.Permissions.Allows(tableProject, principal.ActionRead) {
		projectArm = fmt.Sprintf("EXISTS (SELECT 1 FROM project sj WHERE sj.id = %s.entity_id AND %s)", alias, project("sj"))
	}
	return fmt.Sprintf(`(%[6]s AND (%[1]s.entity_type IS NULL
	 OR (%[1]s.entity_type = 'person'       AND EXISTS (SELECT 1 FROM person sp WHERE sp.id = %[1]s.entity_id AND %[2]s))
	 OR (%[1]s.entity_type = 'company' AND EXISTS (SELECT 1 FROM company so WHERE so.id = %[1]s.entity_id AND %[3]s))
	 OR (%[1]s.entity_type = 'deal'         AND EXISTS (SELECT 1 FROM deal sd WHERE sd.id = %[1]s.entity_id AND %[4]s))
	 OR (%[1]s.entity_type = 'project'      AND %[5]s)))`,
		alias, person("sp"), company("so"), deal("sd"), projectArm, private), nil
}

// EnsureSignalVisible is EnsureVisible for signals, using the
// subject-entity scope above; out of scope reads as ErrNotFound.
func EnsureSignalVisible(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idPos := arg(id)

	clause, err := SignalScopeClause(ctx, "s", arg)
	if err != nil {
		return err
	}
	if clause == "" {
		return nil
	}
	var visible bool
	err = tx.QueryRow(ctx,
		fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM signal s WHERE s.id = $%d AND %s)`, idPos, clause),
		args...).Scan(&visible)
	if err != nil {
		return err
	}
	if !visible {
		return apperrors.ErrNotFound
	}
	return nil
}

// EnsureSignalVisibleLive is EnsureSignalVisible with the two strictnesses a
// caller serving STORED data needs — the row must still be live, and an
// unbounded actor does not skip the probe. See EnsureVisibleLive.
func EnsureSignalVisibleLive(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idPos := arg(id)

	clause, err := SignalScopeClause(ctx, "s", arg)
	if err != nil {
		return err
	}
	return probeExistsLive(ctx, tx, "signal s", "s", idPos, clause, args)
}

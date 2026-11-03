// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"errors"
	"slices"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type (
	mergeAdmissionKey struct{}
	mergeAdmission    struct {
		table             string
		members, grantees []ids.UUID
	}
)

// WithContactCaseAdmission lets a delegated case merge change the selected
// contacts and hand the survivor to one of their owners. It widens
// no other table, no other contact, no read, and no other assignee. The
// caller has already decided the workspace policy and the actor's ownership.
//
// Held by: TestACaseAdmissionWritesOnlyTheSelectedContacts (backend/internal/platform/auth/contactcaseadmission_test.go)
func WithContactCaseAdmission(ctx context.Context, members, grantees []ids.UUID) (context.Context, error) {
	if len(members) < 2 || len(grantees) == 0 {
		return ctx, errors.New("auth: a case admission names at least two contacts and one owner")
	}
	return withMergeAdmission(ctx, mergeAdmission{
		table: tableContact, members: slices.Clone(members), grantees: slices.Clone(grantees),
	})
}

// WithDuplicatePairAdmission lets the merge of one filed duplicate pair change
// both of its records. It widens write authority over those two rows and
// nothing else: no read, no other row, no assignee. The caller has already
// found the pair filed and the actor able to change one end of it.
//
// Held by: TestAPairAdmissionWritesOnlyItsTwoRecords (backend/internal/platform/auth/contactcaseadmission_test.go)
func WithDuplicatePairAdmission(ctx context.Context, table string, left, right ids.UUID) (context.Context, error) {
	if !shareableTables[table] || left.IsZero() || right.IsZero() || left == right {
		return ctx, errors.New("auth: a pair admission names two different records of one shareable table")
	}
	return withMergeAdmission(ctx, mergeAdmission{table: table, members: []ids.UUID{left, right}})
}

// withMergeAdmission refuses to stack admissions, because a second one would
// replace the first and silently narrow a merge already under way.
func withMergeAdmission(ctx context.Context, admission mergeAdmission) (context.Context, error) {
	if _, held := ctx.Value(mergeAdmissionKey{}).(mergeAdmission); held {
		return ctx, errors.New("auth: a merge admission is already in force for this request")
	}
	return context.WithValue(ctx, mergeAdmissionKey{}, admission), nil
}

func admittedCaseMember(ctx context.Context, table string, id ids.UUID) bool {
	admission, ok := ctx.Value(mergeAdmissionKey{}).(mergeAdmission)
	return ok && table == admission.table && slices.Contains(admission.members, id)
}

// admittedCaseMemberFor is admittedCaseMember for one of several principals a
// batch admits: the admission is the acting principal's, never a colleague's.
func admittedCaseMemberFor(ctx context.Context, p principal.Principal, table string, id ids.UUID) bool {
	actor, ok := principal.Actor(ctx)
	return ok && actor.ID == p.ID && admittedCaseMember(ctx, table, id)
}

func admittedCaseGrantee(ctx context.Context, dest ids.UUID) bool {
	admission, ok := ctx.Value(mergeAdmissionKey{}).(mergeAdmission)
	return ok && slices.Contains(admission.grantees, dest)
}

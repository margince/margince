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
	contactCaseKey       struct{}
	contactCaseAdmission struct{ members, grantees []ids.UUID }
)

// WithContactCaseAdmission lets a delegated case merge change exactly the
// selected contacts and hand the survivor to one of their owners. It widens
// no other table, no other contact, no read, and no other assignee. The
// caller has already decided the workspace policy and the actor's ownership.
func WithContactCaseAdmission(ctx context.Context, members, grantees []ids.UUID) (context.Context, error) {
	if len(members) < 2 || len(grantees) == 0 {
		return ctx, errors.New("auth: a case admission names at least two contacts and one owner")
	}
	return context.WithValue(ctx, contactCaseKey{}, contactCaseAdmission{
		members: slices.Clone(members), grantees: slices.Clone(grantees),
	}), nil
}

func admittedCaseMember(ctx context.Context, table string, id ids.UUID) bool {
	admission, ok := ctx.Value(contactCaseKey{}).(contactCaseAdmission)
	return ok && table == "contact" && slices.Contains(admission.members, id)
}

// admittedCaseMemberFor is admittedCaseMember for one of several principals a
// batch admits: the admission is the acting principal's, never a colleague's.
func admittedCaseMemberFor(ctx context.Context, p principal.Principal, table string, id ids.UUID) bool {
	actor, ok := principal.Actor(ctx)
	return ok && actor.ID == p.ID && admittedCaseMember(ctx, table, id)
}

func admittedCaseGrantee(ctx context.Context, dest ids.UUID) bool {
	admission, ok := ctx.Value(contactCaseKey{}).(contactCaseAdmission)
	return ok && slices.Contains(admission.grantees, dest)
}

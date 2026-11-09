// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import "context"

// ActivityDiscoverClause is the weaker activity gate. It answers whether the
// caller may learn that an activity exists (its date, direction, kind and
// owner), not what it says. An activity is discoverable when any record it is
// linked to is visible to the caller, or when it has no links at all.
//
// A reader that shows subject, body, participants, thread or attachments
// composes ActivityContentClause instead. The clause is never empty: an
// unbounded caller skips the link walk but not ActivityAvailableClause.
func ActivityDiscoverClause(ctx context.Context, alias string, arg func(any) int) (string, error) {
	p, err := rbacActor(ctx)
	if err != nil {
		return "", err
	}
	return discoverClauses.render(p, alias, arg, discoverGateFor), nil
}

// ActivityContentClause is the stronger activity gate: the caller may discover
// the activity and is in its audience. The audience is `workspace` (everyone
// who can discover it), `participants` (the humans on it) or `selected`. A
// selected audience is the participants plus activity_audience_member's users
// and teams. Every reader that serves content, or anything derived from it,
// composes this one.
//
// The audience does not yield to row_scope=all, because an admin reading a
// colleague's limited mail is what the limit prevents. Only the system
// principal reads the audience arm away.
func ActivityContentClause(ctx context.Context, alias string, arg func(any) int) (string, error) {
	p, err := rbacActor(ctx)
	if err != nil {
		return "", err
	}
	return contentClauses.render(p, alias, arg, contentGateFor), nil
}

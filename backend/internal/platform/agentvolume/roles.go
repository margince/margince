// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agentvolume

import (
	"context"
	"fmt"
	"slices"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Allowance is the limits one caller is held to, and for each governed counter
// the role whose own value set it. An empty role means the installation value.
type Allowance struct {
	Limits Limits
	From   map[Counter]string
}

// For is the allowance of an agent whose granting human holds roles. Per counter
// it is the highest of those roles' allowances, where a role with no value of
// its own counts as the installation value. A human with no live role gets the
// installation value.
//
// It is derived on every call from the policy and the caller's roles. Nothing
// per caller is cached, so no caller is judged by the roles of the one before.
func (p Policy) For(roles []string) Allowance {
	out := Allowance{From: make(map[Counter]string, len(usageCounters))}
	for _, c := range usageCounters {
		limit, from := p.highest(roles, c)
		out.Limits = out.Limits.With(c, limit)
		out.From[c] = from
	}
	return out
}

// Paced reports whether an agent whose granting human holds roles is held to
// the per-minute pacing. One unpaced role lifts it. Like For, it is derived on
// every call, so no caller is judged by the roles of the one before.
func (p Policy) Paced(roles []string) bool {
	return !slices.ContainsFunc(roles, func(role string) bool {
		return slices.Contains(p.Unpaced, role)
	})
}

// Paced is Policy.Paced for the agent on ctx, judged by the policy the window
// allowances use. Anything but an authenticated agent is paced.
func (m *Meter) Paced(ctx context.Context) bool {
	actor, present := principal.Actor(ctx)
	if !present || actor.Type != principal.PrincipalAgent {
		return true
	}
	return m.policy(ctx).Paced(rolesOf(ctx))
}

// highest walks the roles in key order, so a tie names the same role on every
// call. A role's own value wins a tie only over another role's own value.
func (p Policy) highest(roles []string, c Counter) (limit int, from string) {
	installation := p.Limits.Of(c)
	if len(roles) == 0 {
		return installation, ""
	}
	seen := false
	for _, role := range slices.Sorted(slices.Values(roles)) {
		value, own := installation, ""
		if set := p.Roles[role].Of(c); set > 0 {
			value, own = set, role
		}
		if !seen || value > limit || (value == limit && own == "") {
			limit, from, seen = value, own, true
		}
	}
	return limit, from
}

// With is l with counter c's threshold set to n. Cost has no configured
// threshold, so setting one is a caller defect.
func (l Limits) With(c Counter, n int) Limits {
	switch c {
	case Writes:
		l.Writes = n
	case Egress:
		l.Egress = n
	case Calls:
		l.Calls = n
	default:
		panic(fmt.Sprintf("agentvolume: %s has no configured limit", c))
	}
	return l
}

// rolesOf names the roles of the human behind a governed call: for an agent,
// the granting human's live roles, which authentication loads on every call.
func rolesOf(ctx context.Context) []string {
	actor, present := principal.Actor(ctx)
	if !present || actor.Type != principal.PrincipalAgent {
		return nil
	}
	return actor.Permissions.RoleKeys
}

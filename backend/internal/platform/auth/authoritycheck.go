// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Check is one authority question a gate in this package asks of its caller.
// Require, RequireAny, RequireAnyObject, RequireCoach, RequireRowScope and
// RequireAdmin decide through HeldBy, so a listing that asks HeldBy answers
// what those gates would.
type Check struct {
	// Object is the object type, or anyObject for RequireAnyObject.
	Object string
	// Actions admit when any one of them is granted on Object.
	Actions []principal.Action
	// Admin is RequireAdmin's question: does the caller hold the admin role key.
	Admin bool
	// RowScope is RequireRowScope's question: is the caller's row scope at
	// least this wide.
	RowScope principal.RowScope
}

const (
	anyObject      = "*"
	adminCheckName = "admin"
	rowScopePrefix = "scope:"
)

// String is the check as the generated tool table stores it: "deal.update",
// "deal.create|update", "*.create", "admin" or "scope:team".
func (c Check) String() string {
	if c.Admin {
		return adminCheckName
	}
	if c.RowScope != "" {
		return rowScopePrefix + string(c.RowScope)
	}
	actions := make([]string, len(c.Actions))
	for i, a := range c.Actions {
		actions[i] = string(a)
	}
	return c.Object + "." + strings.Join(actions, "|")
}

// ParseCheck reads a check written by String.
func ParseCheck(s string) (Check, error) {
	if s == adminCheckName {
		return Check{Admin: true}, nil
	}
	if scope, ok := strings.CutPrefix(s, rowScopePrefix); ok {
		c := Check{RowScope: principal.RowScope(scope)}
		if !slices.Contains([]principal.RowScope{principal.RowScopeOwn, principal.RowScopeTeam, principal.RowScopeAll}, c.RowScope) {
			return Check{}, fmt.Errorf("auth: %q names the row scope %q, which is not own, team or all", s, scope)
		}
		return c, nil
	}
	object, actions, ok := strings.Cut(s, ".")
	if !ok || object == "" || actions == "" {
		return Check{}, fmt.Errorf("auth: %q is not object.action, object.a|b, admin or scope:<row scope>", s)
	}
	c := Check{Object: object}
	for word := range strings.SplitSeq(actions, "|") {
		a := principal.Action(word)
		if !slices.Contains([]principal.Action{principal.ActionCreate, principal.ActionRead, principal.ActionUpdate, principal.ActionDelete}, a) {
			return Check{}, fmt.Errorf("auth: %q names the action %q, which is not create, read, update or delete", s, word)
		}
		c.Actions = append(c.Actions, a)
	}
	return c, nil
}

// HeldBy is the gate's decision on c for p. A buyer holds nothing and the
// system principal holds everything, as at every gate in this package.
func (c Check) HeldBy(p principal.Principal) bool {
	switch {
	case p.Type == principal.PrincipalBuyer:
		return false
	case c.RowScope != "":
		// Read for the system too, which arrives on its own row scope.
		return !c.RowScope.Wider(p.Permissions.RowScope)
	case p.Type == principal.PrincipalSystem:
		return true
	case c.Admin:
		return slices.Contains(p.Permissions.RoleKeys, roleAdmin)
	case c.Object == anyObject:
		for object := range p.Permissions.Objects {
			if grantsAnyOf(p.Permissions, object, c.Actions) {
				return true
			}
		}
		return false
	}
	return grantsAnyOf(p.Permissions, c.Object, c.Actions)
}

func grantsAnyOf(perms principal.Permissions, object string, actions []principal.Action) bool {
	return slices.ContainsFunc(actions, func(a principal.Action) bool { return perms.Allows(object, a) })
}

// checkRecorderKey carries a checkRecorder. Only the integration build can put
// one in a context (authoritycheck_integration.go), so a shipped binary never
// records, and no request can make it.
type checkRecorderKey struct{}

// checkRecorder collects the checks asked of one principal during one call.
// A check asked of another principal on the same context is left out: a
// recipient a notice is tested against is not the caller.
type checkRecorder struct {
	actor  string
	mu     sync.Mutex
	checks map[string]bool
}

func noteCheck(ctx context.Context, c Check) {
	rec, ok := ctx.Value(checkRecorderKey{}).(*checkRecorder)
	if !ok {
		return
	}
	if p, ok := principal.Actor(ctx); !ok || p.ID != rec.actor {
		return
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	rec.checks[c.String()] = true
}

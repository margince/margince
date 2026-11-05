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
	// NarrowerThan is RequireRowScopeNarrowerThan's question: is the caller's
	// row scope narrower than this.
	NarrowerThan principal.RowScope
}

const (
	anyObject      = "*"
	adminCheckName = "admin"
	rowScopePrefix = "scope:"
	narrowerPrefix = "scope-narrower-than:"
	// rowRefusalPrefix names a row a gate hid from the caller, as HideRow notes it.
	rowRefusalPrefix = "row:"
)

// String is the check as the generated tool table stores it: "deal.update",
// "deal.create|update", "*.create", "admin", "scope:team" or
// "scope-narrower-than:all".
func (c Check) String() string {
	if c.Admin {
		return adminCheckName
	}
	if c.NarrowerThan != "" {
		return narrowerPrefix + string(c.NarrowerThan)
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
	if scope, ok := strings.CutPrefix(s, narrowerPrefix); ok {
		c := Check{NarrowerThan: principal.RowScope(scope)}
		return c, knownRowScope(s, c.NarrowerThan)
	}
	if scope, ok := strings.CutPrefix(s, rowScopePrefix); ok {
		c := Check{RowScope: principal.RowScope(scope)}
		return c, knownRowScope(s, c.RowScope)
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

func knownRowScope(check string, scope principal.RowScope) error {
	if !slices.Contains([]principal.RowScope{principal.RowScopeOwn, principal.RowScopeTeam, principal.RowScopeAll}, scope) {
		return fmt.Errorf("auth: %q names the row scope %q, which is not own, team or all", check, scope)
	}
	return nil
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
	case c.NarrowerThan != "":
		return c.NarrowerThan.Wider(p.Permissions.RowScope)
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

// checkRecorder collects the checks asked of one principal during one call,
// and the refusals a gate answered with something other than 403. A note about
// another principal on the same context is left out: a recipient a notice is
// tested against is not the caller.
type checkRecorder struct {
	actor   string
	mu      sync.Mutex
	checks  map[string]bool
	refused map[string]bool
}

func noteCheck(ctx context.Context, c Check) {
	note(ctx, func(rec *checkRecorder) { rec.checks[c.String()] = true })
}

// noteRefusal records that a gate refused the caller, for a refusal whose
// answer does not say so: a hidden row reads as not found.
func noteRefusal(ctx context.Context, name string) {
	note(ctx, func(rec *checkRecorder) { rec.refused[name] = true })
}

func note(ctx context.Context, write func(*checkRecorder)) {
	rec, ok := ctx.Value(checkRecorderKey{}).(*checkRecorder)
	if !ok {
		return
	}
	if p, ok := principal.Actor(ctx); !ok || p.ID != rec.actor {
		return
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	write(rec)
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/pkg/extension"
)

// rbacDefaultsExtension is the operation annotation carrying the starting grant
// each system role gets on the operation's RBAC object. It sits beside
// x-rbac-object, so the object cannot be a second name that disagrees.
const rbacDefaultsExtension = "x-rbac-defaults"

// errNoDefaultRole refuses an empty or null block. Written by someone who meant
// to declare defaults, it would pass as "none" and leave the unit unusable.
var errNoDefaultRole = fmt.Errorf("%s declares no role: name a role and its actions, or leave the annotation out", rbacDefaultsExtension)

// rbacDefaultsOf reads the annotation as a role to actions map, in role order.
// The order is sorted because the result is emitted into generated Go, and a
// reflowed fragment must not look like a changed declaration. The boot checks
// that a role key names a system role. Verb.Validate checks the rest.
func rbacDefaultsOf(node *yaml.Node) ([]extension.RbacDefault, error) {
	if node.IsZero() {
		return nil, nil
	}
	if node.Kind == yaml.ScalarNode && node.Tag == "!!null" {
		return nil, errNoDefaultRole
	}
	byRole, err := decodeStrict[map[string][]string](node)
	if err != nil {
		return nil, fmt.Errorf("%s must map a role key to a list of actions: %w", rbacDefaultsExtension, err)
	}
	if len(byRole) == 0 {
		return nil, errNoDefaultRole
	}
	roles := make([]string, 0, len(byRole))
	for role := range byRole {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	out := make([]extension.RbacDefault, 0, len(roles))
	for _, role := range roles {
		actions := make([]extension.RbacAction, 0, len(byRole[role]))
		for _, a := range byRole[role] {
			actions = append(actions, extension.RbacAction(a))
		}
		out = append(out, extension.RbacDefault{Role: role, Actions: actions})
	}
	return out, nil
}

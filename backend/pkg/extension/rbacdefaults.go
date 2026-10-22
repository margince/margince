// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

import (
	"fmt"
	"slices"
	"strings"
)

// SystemRoleKeys are the role keys a default grant may name, in the order the
// seed lays the roles down. It mirrors the roles the identity module seeds,
// which a published surface cannot import. A gate fails when the two differ in
// either direction (backend/gates/systemrolekeys_test.go).
var SystemRoleKeys = []string{"admin", "management", "manager", "rep", "read_only", "ops"}

// RbacDefaultsEqual compares two declarations as sets. The order a unit wrote
// its roles or verbs in is not a different default.
func RbacDefaultsEqual(a, b []RbacDefault) bool {
	if len(a) != len(b) {
		return false
	}
	return slices.Equal(rbacDefaultKeys(a), rbacDefaultKeys(b))
}

func rbacDefaultKeys(ds []RbacDefault) []string {
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		actions := make([]string, 0, len(d.Actions))
		for _, x := range d.Actions {
			actions = append(actions, string(x))
		}
		slices.Sort(actions)
		out = append(out, d.Role+"="+strings.Join(actions, ","))
	}
	slices.Sort(out)
	return out
}

// CheckRbacDefaultsAgree refuses operations that state different default
// grants for one object. The first one read would win silently, and the
// contract would say two things. The generator and the boot both run it.
func CheckRbacDefaultsAgree(verbs []Verb) error {
	declared := map[string]Verb{}
	for _, v := range verbs {
		if len(v.RbacDefaults) == 0 {
			continue
		}
		prior, ok := declared[v.RbacObject]
		if !ok {
			declared[v.RbacObject] = v
			continue
		}
		if !RbacDefaultsEqual(prior.RbacDefaults, v.RbacDefaults) {
			return fmt.Errorf("operations %s and %s both declare default grants for %q and they differ: state them once, or state them identically",
				prior.OperationID, v.OperationID, v.RbacObject)
		}
	}
	return nil
}

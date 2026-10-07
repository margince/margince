// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"fmt"
	"strings"
	"testing"
)

// The group→role map validator's bounds and vocabulary. The allowed values are
// DERIVED from systemRoles, so the accepting case walks that set rather than
// naming keys — a role renamed there stays mappable without touching this test.
func TestValidateGroupRoleMapHoldsItsBoundsAndVocabulary(t *testing.T) {
	everyRole := map[string]string{}
	for _, role := range systemRoles {
		everyRole["dir-"+role.key] = role.key
	}
	if err := validateGroupRoleMap(everyRole); err != nil {
		t.Errorf("a map onto every seeded role was refused: %v", err)
	}
	// Nil is the registered default — never configured — and must validate,
	// or the entry could not be written back to absent.
	if err := validateGroupRoleMap(nil); err != nil {
		t.Errorf("the nil default was refused: %v", err)
	}

	atCeiling := map[string]string{}
	for i := range maxGroupRoleMapEntries {
		atCeiling[fmt.Sprintf("group-%d", i)] = "rep"
	}
	if err := validateGroupRoleMap(atCeiling); err != nil {
		t.Errorf("a map at the %d-entry ceiling was refused: %v", maxGroupRoleMapEntries, err)
	}
	atCeiling["one-more"] = "rep"
	if err := validateGroupRoleMap(atCeiling); err == nil {
		t.Error("a map past the ceiling was accepted")
	}

	for name, m := range map[string]map[string]string{
		"an unknown role key": {"crm-users": "sysadmin"},
		"a blank group":       {"": "rep"},
		"a whitespace group":  {"   ": "rep"},
		"a padded group":      {" sales": "rep"},
		"an over-long group":  {strings.Repeat("g", maxGroupKeyLen+1): "rep"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateGroupRoleMap(m); err == nil {
				t.Errorf("%v was accepted", m)
			}
		})
	}
}

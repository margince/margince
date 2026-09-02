// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The seats that may coach are seats that exist.
//
// platform/auth names the coaching roles by key, because platform sits below
// modules in the DAG and cannot read the module that seeds them. That is the
// architecture, not an oversight — but it leaves two spellings of one
// vocabulary, and the way they drift is silent: rename or retire a seeded role
// and RequireCoach keeps naming the old key, admitting nobody. A gate that only
// answered "does this key exist" would not catch the reverse, so this also
// pins the count: a NEW leadership seat that should coach and does not appear
// in the list is the same defect from the other side, and it is the one a
// reader would not think to look for.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"
)

var (
	coachingRolesDecl = regexp.MustCompile(`var coachingRoles = \[\]string\{([^}]*)\}`)
	roleKeyLiteral    = regexp.MustCompile(`"([a-z_]+)"`)
	seededRoleEntry   = regexp.MustCompile(`\{"([a-z_]+)",\s*"[^"]*"\}`)
	roleAdminDecl     = regexp.MustCompile(`const roleAdmin = "([a-z_]+)"`)
)

func TestTheCoachingRolesAreSeededRoles(t *testing.T) {
	t.Parallel()

	coaching := coachingRoleKeys(t)
	if len(coaching) == 0 {
		t.Fatal("read no coaching roles out of platform/auth — the declaration this gate reads has moved")
	}
	seeded := seededRoleKeys(t)
	if len(seeded) == 0 {
		t.Fatal("read no seeded roles out of identity — the declaration this gate reads has moved")
	}

	for _, role := range coaching {
		if !seeded[role] {
			t.Errorf("platform/auth admits %q to coach, but identity seeds no such role — "+
				"a key that names nobody admits nobody, and the endpoint refuses every caller", role)
		}
	}

	// The reverse, stated as the roles that deliberately do NOT coach. A seeded
	// role missing from BOTH lists is a new seat nobody decided about, which is
	// the failure a one-directional check cannot see.
	notCoaching := map[string]bool{"rep": true, "read_only": true, "ops": true}
	for role := range seeded {
		if notCoaching[role] {
			continue
		}
		if !containsRole(coaching, role) {
			t.Errorf("identity seeds %q, and it neither coaches nor is listed as a seat that does not — "+
				"decide which it is: add it to coachingRoles, or to this test's notCoaching set", role)
		}
	}
}

func containsRole(roles []string, want string) bool {
	i := sort.SearchStrings(roles, want)
	return i < len(roles) && roles[i] == want
}

// coachingRoleKeys reads the keys out of the declaration itself rather than
// importing it: the constant is unexported, and exporting it to be tested would
// widen the package's surface for the test's convenience.
func coachingRoleKeys(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("internal", "platform", "auth", "rbac.go"))
	if err != nil {
		t.Fatalf("reading the rbac gate source: %v", err)
	}
	decl := coachingRolesDecl.FindSubmatch(raw)
	if decl == nil {
		return nil
	}
	// roleAdmin enters the list as a const reference, so its VALUE is read from
	// its own declaration rather than assumed. Spelling "admin" here instead
	// would leave this gate green after somebody changed what that constant
	// holds — the exact drift it exists to catch, in the one key it cannot see.
	adminDecl := roleAdminDecl.FindSubmatch(raw)
	if adminDecl == nil {
		t.Fatal("could not read roleAdmin's value out of rbac.go — the declaration this gate reads has moved")
	}
	keys := []string{string(adminDecl[1])}
	for _, m := range roleKeyLiteral.FindAllSubmatch(decl[1], -1) {
		keys = append(keys, string(m[1]))
	}
	sort.Strings(keys)
	return keys
}

func seededRoleKeys(t *testing.T) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("internal", "modules", "identity", "service.go"))
	if err != nil {
		t.Fatalf("reading the identity service source: %v", err)
	}
	keys := map[string]bool{}
	for _, m := range seededRoleEntry.FindAllSubmatch(raw, -1) {
		keys[string(m[1])] = true
	}
	return keys
}

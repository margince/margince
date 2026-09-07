// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The screens ask what a seat MAY DO, not which role it holds.
//
// /me carries the server's own answer — the object grants and the row scope it
// computed from the stored policy — precisely so a screen never re-derives one
// from role keys. A client that reads the keys instead is a second reading of
// the policy, and the two disagree exactly where it matters: an operator edits
// a seeded role's grants or its row scope (the role editor allows both), or an
// installation adds a role of its own, and the server answers correctly while
// the screen shows the wrong view.
//
// This is not hypothetical. The lead queue decided its opening view from
// `roles.some(r => r === "admin" || "manager" || "management")` until #4728,
// which is the shape this gate now refuses.
//
// The vocabulary is READ FROM THE SEEDED ROLES rather than typed here, so a
// renamed or added role is covered without editing this file — a hard-coded
// list would be a second copy of the thing it protects.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// capabilityModule is the one file allowed to read role keys. It holds the
// two fixed-role helpers (useHoldsAdminRole, useHoldsOperatorSeat) that cover
// authorities the RbacObject vocabulary does not name yet, and it documents
// each as exceptional. Every other screen asks it, or asks useCan.
const capabilityModule = "src/app/capability.ts"

// roleKeyComparison matches a role key being COMPARED, not merely mentioned.
//
// Comparison and not presence, because the keys are legitimate vocabulary in
// other roles: users-admin.tsx renders them as a typed picker, i18n names them,
// and settings.tsx carries a navigation group that happens to be spelled
// "admin". None of those decides an authorization question. What does is a key
// tested against a value — which is what these three shapes are.
func roleKeyComparisons(src string, keys []string) []string {
	var found []string
	for _, key := range keys {
		for _, shape := range []*regexp.Regexp{
			// role === "admin", "admin" === role, and the !== halves. The left
			// operand is captured so a comparison about something ELSE spelled
			// with the same word can be told apart below.
			regexp.MustCompile(`(\w*)\s*[!=]==\s*"` + key + `"`),
			regexp.MustCompile(`"` + key + `"\s*[!=]==\s*(\w*)`),
			// roles.includes("admin"); .some(r => r === "admin") is caught above.
			regexp.MustCompile(`(\w*)\.includes\(\s*"` + key + `"\s*\)`),
		} {
			for _, m := range shape.FindAllStringSubmatch(src, -1) {
				if decidesAboutARole(m[1]) {
					found = append(found, key)
					break
				}
			}
		}
	}
	return found
}

// decidesAboutARole says whether the compared operand is a PRINCIPAL's role.
//
// The seeded keys are ordinary English words, and two other vocabularies in
// this tree spell one of them the same way: the settings navigation groups its
// pages as "you" or "admin", and a buying committee seat can be a "champion".
// Neither decides an authorization question, and failing them would teach the
// next reader that this gate cries wolf.
//
// So the operand has to name the principal's roles. An unnamed operand — the
// comparison is against a call, an index, a property chain — is treated as a
// role decision, because that is the direction a census must fail in: an
// under-recognizing scan reports PASS and nothing is there to notice.
func decidesAboutARole(operand string) bool {
	switch operand {
	case "group", "kind", "type", "id", "tab", "mode", "variant", "status":
		return false
	}
	return true
}

func TestNoScreenDecidesAuthorizationFromARoleKey(t *testing.T) {
	t.Parallel()

	keys := seededRoleKeyList(t)
	if len(keys) == 0 {
		t.Fatal("read no seeded role keys out of identity — the declaration this gate reads has moved")
	}

	// The gate must bite. A key every screen legitimately mentions but never
	// compares would make this scan pass over a tree it never really read, so
	// the shapes are pinned against text that must match and text that must not.
	mustMatch := []string{
		`role === "admin"`,
		`session.roles.some((r) => r === "manager")`,
		`roles.includes("ops")`,
		`if (role !== "rep") {`,
	}
	for _, sample := range mustMatch {
		if len(roleKeyComparisons(sample, keys)) == 0 {
			t.Errorf("the scan does not recognize %q as a role comparison, so it would pass a screen that decides from one", sample)
		}
	}
	mustNotMatch := []string{
		`group: "admin"`, // settings navigation, not a role
		`const ROLES: readonly Role[] = ["admin", "ops"]`, // the typed picker vocabulary
		`t("role.admin")`,          // an i18n key
		`seat.role === "champion"`, // a buying-committee role, a different vocabulary
	}
	for _, sample := range mustNotMatch {
		if hits := roleKeyComparisons(sample, keys); len(hits) > 0 {
			t.Errorf("the scan reads %q as a role comparison (%v), which would fail a screen that decides nothing", sample, hits)
		}
	}

	scanned := 0
	err := filepath.WalkDir("../frontend/src", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".ts") && !strings.HasSuffix(name, ".tsx") {
			return nil
		}
		// Tests and stories BUILD principals rather than decide about them: a
		// fixture saying roles: ["admin"] is the input to the thing under test.
		if strings.Contains(name, ".test.") || strings.Contains(name, ".stories.") {
			return nil
		}
		rel, relErr := filepath.Rel("../frontend", path)
		if relErr != nil {
			return relErr
		}
		if filepath.ToSlash(rel) == capabilityModule {
			return nil
		}
		src, readErr := os.ReadFile(path) // #nosec G304 -- a .ts file from walking the trusted source tree
		if readErr != nil {
			return readErr
		}
		scanned++
		if hits := roleKeyComparisons(string(src), keys); len(hits) > 0 {
			sort.Strings(hits)
			t.Errorf("%s compares the role key(s) %v.\n"+
				"A screen decides from what /me says the seat MAY DO — `useCan(object, action)` for an\n"+
				"object grant, `authorization.row_scope` for the row question — never from the role the\n"+
				"grant came from. The two disagree as soon as an operator edits a role or adds one.\n"+
				"An authority the RbacObject vocabulary cannot name yet belongs in %s beside the other\n"+
				"exceptions, with a comment saying why and what retires it.",
				filepath.ToSlash(rel), hits, capabilityModule)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the frontend source: %v", err)
	}
	// A scan that read nothing reports PASS, which is the one way this gate
	// could fail silently: the tree moved and nobody was told.
	if scanned < 200 {
		t.Fatalf("scanned only %d frontend sources — the tree has moved and this gate is reading the wrong place", scanned)
	}
}

// seededRoleKeyList is seededRoleKeys as a stable slice, so the failure message
// and the scan order do not depend on map iteration.
func seededRoleKeyList(t *testing.T) []string {
	t.Helper()
	keys := make([]string, 0, 6)
	for key := range seededRoleKeys(t) {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

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
// a seeded role's grants, or an installation adds a role of its own, and the
// server answers correctly while the screen shows the wrong view.
//
// This is not hypothetical. The lead queue decided its opening view from
// `roles.some(r => r === "admin" || "manager" || "management")` until the change
// that added this gate, which is the shape it now refuses.
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
		if roleKeyIsDecidedOn(src, key) {
			found = append(found, key)
		}
	}
	return found
}

// roleKeyIsDecidedOn answers whether this source DECIDES on the key.
//
// The scan is textual, so the question is which spellings it can be sure about.
// Two families:
//
// The QUOTED-COMPARISON family is a key compared where it is written —
// `role === "admin"`, a `case "admin":`, a template literal, an indexOf, an
// array of keys tested for membership. Each is a decision unless the compared
// operand names something that is not a role, which is the second function
// below.
//
// The BOUND-KEY family is a key bound to a name first — `const ADMIN = "admin"`
// — and compared against that name later. The comparison carries no literal, so
// it is found in two steps: bind the name, then look for that name being
// compared against something that is not exempt. Binding alone is NOT the
// finding, because a URL segment is also spelled "admin" (settingsrouting.ts's
// LEGACY_ADMIN_SEGMENT), and failing a constant nobody decides a role with is
// the noise that gets a gate ignored.
func roleKeyIsDecidedOn(src, key string) bool {
	quoted := "(?:\"" + key + "\"|'" + key + "'|`" + key + "`)"
	for _, shape := range []*regexp.Regexp{
		// role === "admin" / role == "admin", and the negated halves. The
		// operand is captured so a comparison about something else spelled the
		// same way can be told apart.
		regexp.MustCompile(`(?:\w+[.?]+)*(\w*)\s*[!=]==?\s*` + quoted),
		regexp.MustCompile(quoted + `\s*[!=]==?\s*(?:\w+[.?]+)*(\w*)`),
		// roles.includes("admin"), roles.indexOf("admin"), and the same two
		// with the array on the literal side: ["admin","ops"].includes(role).
		regexp.MustCompile(`(\w*)\.(?:includes|indexOf)\(\s*` + quoted),
		regexp.MustCompile(quoted + `[^)\n]*\]\s*\.(?:includes|indexOf)\(\s*(\w*)`),
		// switch (role) { case "admin": — the operand is on the switch line, so
		// it cannot be read here. A case arm naming a role key is a decision.
		regexp.MustCompile(`case\s+` + quoted + `\s*:()`),
	} {
		for _, m := range shape.FindAllStringSubmatch(src, -1) {
			if decidesAboutARole(m[len(m)-1]) {
				return true
			}
		}
	}
	return boundKeyIsComparedOn(src, quoted)
}

// boundKeyIsComparedOn finds the two-step spelling: the key bound to a name,
// then that name compared. Both halves are required — the binding says which
// name to look for, and the comparison says the name decides something.
func boundKeyIsComparedOn(src, quoted string) bool {
	binding := regexp.MustCompile(`(?:const|let|var)\s+(\w+)\s*(?::[^=\n]+)?=\s*` + quoted + `\s*[;,\n]`)
	for _, bound := range binding.FindAllStringSubmatch(src, -1) {
		name := bound[1]
		for _, shape := range []*regexp.Regexp{
			regexp.MustCompile(`(?:\w+[.?]+)*(\w*)\s*[!=]==?\s*` + name + `\b`),
			regexp.MustCompile(`\b` + name + `\s*[!=]==?\s*(?:\w+[.?]+)*(\w*)`),
			regexp.MustCompile(`(\w*)\.(?:includes|indexOf)\(\s*` + name + `\s*\)`),
		} {
			for _, m := range shape.FindAllStringSubmatch(src, -1) {
				if decidesAboutARole(m[len(m)-1]) {
					return true
				}
			}
		}
	}
	return false
}

// decidesAboutARole says whether the compared operand is a PRINCIPAL's role.
//
// The seeded keys are ordinary English words, and another vocabulary in this
// tree spells one of them the same way: the settings navigation groups its
// pages as "you" or "admin" (settingsnav.tsx). That comparison decides which
// heading a settings page sits under and no authorization question, and failing
// it would teach the next reader that this gate cries wolf.
//
// So ONE name is exempt, and it is the one the tree actually uses. The list is
// deliberately not a catalogue of plausible discriminators: every name added
// here is a name a role decision can hide behind, and `kind => kind === "admin"`
// inside a roles.some() is a real decision that an over-broad list would miss.
// An unnamed operand — the comparison is against a call, an index, a property
// chain, or a case arm — is treated as a role decision, because that is the
// direction a census must fail in: an under-recognizing scan reports PASS and
// nothing is there to notice.
func decidesAboutARole(operand string) bool {
	switch operand {
	// The settings navigation's group ("you" or "admin"), and a route segment
	// compared against route.id. Both are spelled like a role and decide a
	// PAGE, not an authority. These two names are the tree's real collisions;
	// the list is deliberately short, because every name on it is a name a role
	// decision could hide behind.
	case "group", "id":
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
		// The spellings a quoted-comparison-only scan would miss. Each one is a
		// real way to write the prohibited decision, and each was green against
		// an earlier version of this gate.
		"const ADMIN = \"admin\";\nif (role === ADMIN) {",
		"switch (role) {\n      case \"management\":",
		"role == \"admin\"",
		"role === `admin`",
		`roles.indexOf("admin") >= 0`,
		`["admin", "manager"].includes(role)`,
		// A callback parameter is an ordinary name, so naming it after a
		// discriminator must not buy an exemption.
		`session.roles.some((kind) => kind === "admin")`,
	}
	for _, sample := range mustMatch {
		if len(roleKeyComparisons(sample, keys)) == 0 {
			t.Errorf("the scan does not recognize %q as a role decision, so it would pass a screen that makes one", sample)
		}
	}
	mustNotMatch := []string{
		// The settings navigation group: the one real collision in this tree.
		`group: "admin"`,
		`entry?.group === "admin"`,
		`entry.group !== "admin" || adminTabVisible[entry.id]`,
		// A key rendered rather than decided on. The picker vocabulary in
		// users-admin.tsx is this shape, and it is type-checked against the
		// contract enum, so a retired key there is a compile error already.
		`const ROLES: readonly Role[] = ["admin", "ops"]`,
		`t("role.admin")`,
		// A URL segment that happens to be spelled like a role, bound and then
		// compared against a ROUTE. settingsrouting.ts is exactly this.
		"const LEGACY_ADMIN_SEGMENT = \"admin\";\nif (route.id === LEGACY_ADMIN_SEGMENT) {",
	}
	for _, sample := range mustNotMatch {
		if hits := roleKeyComparisons(sample, keys); len(hits) > 0 {
			t.Errorf("the scan reads %q as a role decision (%v), which would fail a screen that decides nothing", sample, hits)
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
	if scanned < 600 {
		t.Fatalf("scanned only %d frontend sources, of roughly 690 eligible — the tree has moved and this gate "+
			"is reading part of it. The floor is close to the real count on purpose: losing one directory "+
			"has to fail here rather than pass quietly", scanned)
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

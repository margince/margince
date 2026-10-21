// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// The enabled set must be a set git actually has.
//
// .gitignore carries `/extensions/*` with a per-unit un-ignore list, so a new
// first-party unit is IGNORED BY DEFAULT. That is the right default — the
// directory is the installation-owned enabled set, and an operator's units are
// not this repository's business — but it means a contributor adding a unit
// meant to ship in the vanilla tree has to remember one line in a file they
// were not otherwise editing.
//
// Nothing caught it. `make composition`, `make migrate`,
// `make check-ext-migrations` and the whole of `make check` read the WORKING
// TREE, so every one of them is green over a unit that no clone of this
// repository has: the composition composes it, the migration gate applies its
// SQL, its tests run — and `git add extensions/<name>/` adds nothing, the
// commit lands with the directory absent, and CI builds a vanilla tree without
// it. The shipped artifact and the gated artifact are two different things,
// which is the exact failure class a fitness test exists for. notes hit it;
// docs/how-to/add-an-extension.md had already warned about it in prose, and
// prose was not enough.
//
// It asks git rather than parsing .gitignore because git is the authority: the
// pattern language has precedence rules (a later negation, a directory-level
// .gitignore, .git/info/exclude) that a reimplementation would get subtly
// wrong, and being subtly wrong here is indistinguishable from passing.

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestEveryEnabledExtensionIsTracked: no first-party directory under
// extensions/ may be ignored.
//
// A unit's go.mod says whose it is. This repository's units are
// github.com/margince/margince/extensions/<name>, as the how-to prescribes; a
// unit under any other path is an installation's own, staged into its checkout
// of the core and meant to stay ignored. So an ignored unit fails here unless
// its module line names another owner, and a tracked one fails unless it names
// this repository: the second half is what keeps the first from being waved
// through by a module line nobody checks.
//
// It covers fixtures/extensions/ too, whatever their module paths: the fixtures
// are what CI copies into extensions/ to exercise the tier, so a fixture git
// does not have is the same defect one lane further out.
func TestEveryEnabledExtensionIsTracked(t *testing.T) {
	t.Parallel()
	namespace := repositoryModuleNamespace(t)
	for _, root := range []string{"../extensions", "../fixtures/extensions"} {
		entries, err := os.ReadDir(root)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue // .gitkeep, approvals.lock
			}
			// The check-ignore question is asked about a FILE inside the unit,
			// not the directory: `/extensions/*` matches the directory itself,
			// and a negation that re-included the directory without its
			// contents would still leave every source file ignored. A unit
			// always has a go.mod — scanUnit refuses one without it — so this
			// names a path that must exist.
			onDisk := filepath.Join(root, e.Name(), "go.mod")
			if _, err := os.Stat(onDisk); err != nil {
				continue // a unit with no Go module; nothing to be silently dropped
			}
			// git runs at the repository root (-C ..), so it is asked about a
			// root-relative path rather than this package's ../ spelling.
			repoPath := strings.TrimPrefix(filepath.ToSlash(onDisk), "../")
			rule, ignored := gitIgnoreRule(t, repoPath)
			if root == "../fixtures/extensions" {
				if ignored {
					t.Errorf("%s is git-ignored by %q, so CI cannot copy the fixture in.", repoPath, rule)
				}
				continue
			}
			module, firstParty := declaredModulePath(t, onDisk), namespace+"/extensions/"+e.Name()
			switch {
			case ignored && !installationModule(module, namespace):
				t.Errorf("%s is git-ignored by %q — every gate would pass over this unit and no clone "+
					"of this repository would have it. Add an un-ignore line for it beside the others "+
					"in .gitignore (see docs/how-to/add-an-extension.md).", repoPath, rule)
			case !ignored && module != firstParty:
				t.Errorf("%s declares module %q; a unit this repository carries is %q. Any other path "+
					"marks the unit an installation's own, and this gate stops asking whether git has it.",
					repoPath, module, firstParty)
			}
		}
	}
}

// installationModule reports whether a unit's module line names an owner other
// than this repository. An unreadable or missing line names none, so the unit
// is still asked about: the safe direction.
func installationModule(module, namespace string) bool {
	return module != "" && module != namespace && !strings.HasPrefix(module, namespace+"/")
}

// repositoryModuleNamespace is this repository's module prefix, read from the
// backend's own go.mod rather than spelled here.
func repositoryModuleNamespace(t *testing.T) string {
	t.Helper()
	backend := declaredModulePath(t, "go.mod")
	namespace, found := strings.CutSuffix(backend, "/backend")
	if !found {
		t.Fatalf("the backend module is %q, not <namespace>/backend; this gate cannot tell "+
			"this repository's units from an installation's", backend)
	}
	return namespace
}

// declaredModulePath reads a go.mod's module line, or "" when the file cannot be
// read or declares none.
func declaredModulePath(t *testing.T, goMod string) string {
	t.Helper()
	raw, err := os.ReadFile(goMod) // #nosec G304 -- a path this test walked to
	if err != nil {
		return ""
	}
	for line := range strings.SplitSeq(string(raw), "\n") {
		if path, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(path)
		}
	}
	return ""
}

// gitIgnoreRule reports the rule ignoring path, if any. `git check-ignore -v`
// exits 1 when nothing matches, which is the ordinary answer here and not a
// failure — every other exit status is.
func gitIgnoreRule(t *testing.T, path string) (rule string, ignored bool) {
	t.Helper()
	// -C .. : the tests run from backend/, the repository root is one level up.
	out, err := exec.Command("git", "-C", "..", "check-ignore", "-v", "--no-index", path).Output()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return strings.TrimSpace(string(out)), true
	case errors.As(err, &exit) && exit.ExitCode() == 1:
		return "", false
	default:
		detail := err.Error()
		if exit != nil {
			detail = strings.TrimSpace(string(exit.Stderr))
		}
		t.Fatalf("git check-ignore %s: %v — this gate cannot be skipped: the failure it catches is "+
			"invisible to every other check in the tree", path, detail)
		return "", false
	}
}

// TestAUnitNamesItsOwnerByItsModulePath holds the line the gate draws: only a
// module outside this repository's namespace marks a unit an installation's.
func TestAUnitNamesItsOwnerByItsModulePath(t *testing.T) {
	t.Parallel()
	const namespace = "github.com/margince/margince"
	for module, installation := range map[string]bool{
		"github.com/margince/margince/extensions/de": false,
		"github.com/margince/margince":               false,
		"":                                           false,
		"github.com/margince/margince-fork/extensions": true,
		"example.margince.dev/ext/crm-hello":           true,
		"github.com/acme/crm/extensions/billing":       true,
	} {
		if got := installationModule(module, namespace); got != installation {
			t.Errorf("installationModule(%q) = %v, want %v", module, got, installation)
		}
	}
}

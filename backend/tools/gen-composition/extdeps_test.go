// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const lockedDateFns = "      date-fns:\n        specifier: ^4.1.0\n        version: 4.1.0(react@19.2.0)\n"

// writeCoreLockfile writes the repository root's lockfile as pnpm does: one
// document for the package manager, then the project lock holding the importers.
func writeCoreLockfile(t *testing.T, root, deps string) {
	t.Helper()
	lock := "lockfileVersion: '9.0'\n\nimporters:\n\n  .:\n    configDependencies: {}\n\n---\n" +
		"lockfileVersion: '9.0'\n\nimporters:\n\n  frontend:\n    dependencies:\n" + deps +
		"    devDependencies:\n      vitest:\n        specifier: ^5.0.0\n        version: 5.0.0\n"
	if err := os.WriteFile(filepath.Join(root, lockfileName), []byte(lock), 0o644); err != nil {
		t.Fatal(err)
	}
}

// pinFor lays a unit with the given manifest fragment under root and runs the
// same two steps scanExtensions runs on it.
func pinFor(t *testing.T, root, fragment string) (*unitFrontend, error) {
	t.Helper()
	dir := filepath.Join(root, "extensions", "demo")
	writeFrontendLayer(t, dir, `{"name":"@margince-ext/demo","private":true,"main":"screen.tsx",`+fragment+`}`)
	frontend, err := collectUnitFrontend("demo", dir)
	if err != nil {
		return nil, err
	}
	unit := extensionUnit{Name: "demo", Frontend: frontend}
	return frontend, pinUnitRuntimeDeps(root, &unit)
}

func TestUnitRuntimePackagesMustBeTheMainAppsOwn(t *testing.T) {
	for name, tc := range map[string]struct{ fragment, want string }{
		"a dependency the lockfile lacks": {
			fragment: `"dependencies":{"left-pad":"^1.3.0"}`,
			want:     "needs left-pad, which the main app does not use",
		},
		"a peer the lockfile lacks, because pnpm installs peers": {
			fragment: `"peerDependencies":{"left-pad":"^1.3.0"}`,
			want:     "needs left-pad, which the main app does not use",
		},
		"a dev-only package of the core is not shipped": {
			fragment: `"dependencies":{"vitest":"^5.0.0"}`,
			want:     "needs vitest, which the main app does not use",
		},
		"a range the core does not declare": {
			fragment: `"dependencies":{"date-fns":"^4.0.0"}`,
			want:     "the main app locks 4.1.0",
		},
		"a range the locked version does not satisfy": {
			fragment: `"dependencies":{"date-fns":"^3.0.0"}`,
			want:     "the main app locks 4.1.0",
		},
		"an optional dependency is refused outright": {
			fragment: `"optionalDependencies":{"date-fns":"^4.1.0"}`,
			want:     "optionalDependencies",
		},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeCoreLockfile(t, root, lockedDateFns)
			_, err := pinFor(t, root, tc.fragment)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want one mentioning %q", err, tc.want)
			}
		})
	}
}

func TestUnitRuntimePackagesTheMainAppShipsAreAccepted(t *testing.T) {
	for name, fragment := range map[string]string{
		"the core's own range":               `"dependencies":{"date-fns":"^4.1.0"}`,
		"the locked version":                 `"dependencies":{"date-fns":"4.1.0"}`,
		"a peer the core ships":              `"peerDependencies":{"date-fns":"^4.1.0"}`,
		"host-linked peers and dev packages": `"peerDependencies":{"@margince/frontend":"workspace:*","react":"^19.0.0"},"devDependencies":{"left-pad":"^1.3.0"}`,
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			writeCoreLockfile(t, root, lockedDateFns)
			if _, err := pinFor(t, root, fragment); err != nil {
				t.Fatalf("refused: %v", err)
			}
		})
	}
}

// A lockfile a unit ships is never read, so it cannot approve its own package.
func TestAUnitsOwnLockfileApprovesNothing(t *testing.T) {
	root := t.TempDir()
	writeCoreLockfile(t, root, lockedDateFns)
	own := filepath.Join(root, "extensions", "demo", "frontend")
	if err := os.MkdirAll(own, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(own, lockfileName),
		[]byte("importers:\n  frontend:\n    dependencies:\n      left-pad:\n        specifier: ^1.3.0\n        version: 1.3.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := pinFor(t, root, `"dependencies":{"left-pad":"^1.3.0"}`)
	if err == nil || !strings.Contains(err.Error(), "needs left-pad") {
		t.Fatalf("err = %v, want left-pad refused", err)
	}
}

func TestComposedWorkspacePinsUnitRuntimePackagesToTheLockedVersion(t *testing.T) {
	root := t.TempDir()
	writeCoreLockfile(t, root, lockedDateFns)
	frontend, err := pinFor(t, root, `"dependencies":{"date-fns":"^4.1.0"}`)
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := emitComposedFrontendWorkspace(out, []string{"extensions/demo"}, frontend.Pins); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(out, "pnpm-workspace.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if want := `"date-fns": "4.1.0"`; !strings.Contains(string(raw), want) {
		t.Fatalf("pnpm-workspace.yaml lacks the override %s:\n%s", want, raw)
	}
}

// The build reads the repository's own lockfile, so a package the core ships
// must be accepted against it, not only against a fixture.
func TestTheRepositoryLockfileAcceptsAPackageTheCoreShips(t *testing.T) {
	repo, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	core, err := readCoreRuntimeDeps(repo)
	if err != nil {
		t.Fatal(err)
	}
	geist, ok := core["@fontsource/geist"]
	if !ok {
		t.Fatalf("the core lockfile has no @fontsource/geist; read %d packages", len(core))
	}
	unit := extensionUnit{Name: "demo", Frontend: &unitFrontend{RuntimeDeps: map[string]string{"@fontsource/geist": geist.Specifier}}}
	if err := pinUnitRuntimeDeps(repo, &unit); err != nil {
		t.Fatalf("refused a package the core ships: %v", err)
	}
	if unit.Frontend.Pins["@fontsource/geist"] != geist.Version {
		t.Fatalf("pins = %v, want the locked %s", unit.Frontend.Pins, geist.Version)
	}
}

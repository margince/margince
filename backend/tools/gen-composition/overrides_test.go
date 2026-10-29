// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	coreRail   = "export function WorkspaceRail() { return null; }\n"
	railTarget = "src/app/shellrail.tsx"
)

func writeTreeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// overrideTree is a core with a sidebar rail and a unit "acme" that replaces
// it, locked to core's current file.
func overrideTree(t *testing.T) (string, extensionUnit) {
	t.Helper()
	root := t.TempDir()
	writeTreeFile(t, filepath.Join(root, "frontend", railTarget), coreRail)
	u := overrideUnit(t, root, "acme")
	writeTreeFile(t, filepath.Join(u.Dir, overridesLayer, railTarget), "export * from \"@margince-core/src/app/shellrail\";\n")
	writeTreeFile(t, filepath.Join(u.Dir, overridesLockFile), lockedRail(t, root)+"\n")
	return root, u
}

func overrideUnit(t *testing.T, root, name string) extensionUnit {
	t.Helper()
	return extensionUnit{
		Name:     name,
		Dir:      filepath.Join(root, "extensions", name),
		Frontend: &unitFrontend{Package: extPackagePrefix + name, Export: extPackagePrefix + name},
	}
}

func lockedRail(t *testing.T, root string) string {
	t.Helper()
	sum, err := fileSHA256(filepath.Join(root, "frontend", railTarget))
	if err != nil {
		t.Fatal(err)
	}
	return railTarget + " " + sum
}

func TestOverrideLockedToTheCurrentCoreFileReplacesIt(t *testing.T) {
	root, u := overrideTree(t)

	got, err := collectOverrides(root, []extensionUnit{u})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Replaced) != 1 || got.Replaced[0].Target != railTarget || got.Replaced[0].Unit != "acme" {
		t.Fatalf("got %+v, want acme replacing %s", got.Replaced, railTarget)
	}
	if len(got.Warnings) != 0 {
		t.Fatalf("warnings = %q, want none for a current lock", got.Warnings)
	}
}

// A lock that does not match core warns and still composes: a pinned unit must
// not stop a core upgrade.
func TestLockDriftWarnsAndStillReplaces(t *testing.T) {
	cases := map[string]struct {
		edit func(t *testing.T, root string, u extensionUnit)
		want string
	}{
		"no lock line": {
			edit: func(t *testing.T, _ string, u extensionUnit) {
				writeTreeFile(t, filepath.Join(u.Dir, overridesLockFile), "")
			},
			want: "names no hash for " + railTarget,
		},
		"core file changed since the lock": {
			edit: func(t *testing.T, root string, _ extensionUnit) {
				writeTreeFile(t, filepath.Join(root, "frontend", railTarget), coreRail+"// a later core release\n")
			},
			want: "changed since the override was written",
		},
		"lock pins a file the unit does not replace": {
			edit: func(t *testing.T, root string, u extensionUnit) {
				writeTreeFile(t, filepath.Join(u.Dir, overridesLockFile), lockedRail(t, root)+"\nsrc/app/old.tsx sha256:00\n")
			},
			want: "pins src/app/old.tsx",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			root, u := overrideTree(t)
			tc.edit(t, root, u)
			got, err := collectOverrides(root, []extensionUnit{u})
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Replaced) != 1 {
				t.Fatalf("replaced = %+v, want the rail still replaced", got.Replaced)
			}
			if len(got.Warnings) != 1 || !strings.Contains(got.Warnings[0], tc.want) {
				t.Fatalf("warnings = %q, want one containing %q", got.Warnings, tc.want)
			}
		})
	}
}

func TestScreenUnderCustomIsAddedWithoutALock(t *testing.T) {
	root := t.TempDir()
	u := overrideUnit(t, root, "acme")
	writeTreeFile(t, filepath.Join(u.Dir, overridesLayer, "src/custom/acme-desk/screen.tsx"), "export const screen = {};\n")

	got, err := collectOverrides(root, []extensionUnit{u})
	if err != nil {
		t.Fatal(err)
	}
	want := addedScreen{Unit: "acme", Target: "src/custom/acme-desk/screen.tsx", Source: "extensions/acme/frontend/overrides/src/custom/acme-desk/screen.tsx"}
	if len(got.Added) != 1 || got.Added[0] != want || len(got.Replaced) != 0 {
		t.Fatalf("got %+v, want only %+v added", got, want)
	}
}

func TestOverrideRefusals(t *testing.T) {
	cases := map[string]struct {
		edit func(t *testing.T, root string, u extensionUnit) []extensionUnit
		want string
	}{
		"a new file that is not a custom screen": {
			edit: func(t *testing.T, root string, u extensionUnit) []extensionUnit {
				writeTreeFile(t, filepath.Join(u.Dir, overridesLayer, "src/app/gone.tsx"), "")
				return []extensionUnit{u}
			},
			want: "which this core does not have",
		},
		"a custom folder outside the name grammar": {
			edit: func(t *testing.T, root string, u extensionUnit) []extensionUnit {
				writeTreeFile(t, filepath.Join(u.Dir, overridesLayer, "src/custom/Desk/screen.tsx"), "")
				return []extensionUnit{u}
			},
			want: "which this core does not have",
		},
		"a core file outside the replaceable roots": {
			edit: func(t *testing.T, root string, u extensionUnit) []extensionUnit {
				writeTreeFile(t, filepath.Join(root, "frontend/src/api/client.ts"), "")
				writeTreeFile(t, filepath.Join(u.Dir, overridesLayer, "src/api/client.ts"), "")
				return []extensionUnit{u}
			},
			want: "only files under frontend/src/app/",
		},
		"two units, one file": {
			edit: func(t *testing.T, root string, u extensionUnit) []extensionUnit {
				line := lockedRail(t, root) + "\n"
				other := overrideUnit(t, root, "beta")
				writeTreeFile(t, filepath.Join(other.Dir, overridesLayer, railTarget), "")
				writeTreeFile(t, filepath.Join(other.Dir, overridesLockFile), line)
				return []extensionUnit{u, other}
			},
			want: "acme and beta both override frontend/" + railTarget,
		},
		"two units, one added screen": {
			edit: func(t *testing.T, root string, u extensionUnit) []extensionUnit {
				writeTreeFile(t, filepath.Join(u.Dir, overridesLayer, "src/custom/desk/screen.tsx"), "")
				other := overrideUnit(t, root, "beta")
				writeTreeFile(t, filepath.Join(other.Dir, overridesLayer, "src/custom/desk/screen.tsx"), "")
				return []extensionUnit{u, other}
			},
			want: "acme and beta both override frontend/src/custom/desk/screen.tsx",
		},
		"a lock pinning one file twice": {
			edit: func(t *testing.T, root string, u extensionUnit) []extensionUnit {
				writeTreeFile(t, filepath.Join(u.Dir, overridesLockFile), lockedRail(t, root)+"\n"+railTarget+" sha256:00\n")
				return []extensionUnit{u}
			},
			want: railTarget + " is pinned twice",
		},
		"a screen core's fork already has": {
			edit: func(t *testing.T, root string, u extensionUnit) []extensionUnit {
				writeTreeFile(t, filepath.Join(root, "frontend/src/custom/desk/screen.tsx"), "")
				writeTreeFile(t, filepath.Join(u.Dir, overridesLayer, "src/custom/desk/screen.tsx"), "")
				return []extensionUnit{u}
			},
			want: "core's fork already has frontend/src/custom/desk/screen.tsx",
		},
		"a unit with no frontend package": {
			edit: func(_ *testing.T, _ string, u extensionUnit) []extensionUnit {
				u.Frontend = nil
				return []extensionUnit{u}
			},
			want: "needs frontend/package.json",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			root, u := overrideTree(t)
			_, err := collectOverrides(root, tc.edit(t, root, u))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

// The registry imports an added screen relative to where it is written. It
// keys the screen by its file, so a key clash names the unit.
// Each file that takes a credential or a consent is refused by name, so
// widening replaceableRoots fails this test before it admits one.
func TestCredentialAndConsentFilesStayCores(t *testing.T) {
	for _, target := range []string{
		"src/app/queryclient.ts",
		"src/components/auth-core.tsx",
		"src/features/privacy/erasure.tsx",
		"src/features/settings/settings-passports.tsx",
		"src/features/settings/webhooks.secret.tsx",
		"src/features/sign-in/auth-login.tsx",
		"src/features/sign-in/installation-setup.ai-key.tsx",
	} {
		t.Run(target, func(t *testing.T) {
			root := t.TempDir()
			writeTreeFile(t, filepath.Join(root, "frontend", target), coreRail)
			u := overrideUnit(t, root, "acme")
			writeTreeFile(t, filepath.Join(u.Dir, overridesLayer, target), "")
			sum, err := fileSHA256(filepath.Join(root, "frontend", target))
			if err != nil {
				t.Fatal(err)
			}
			writeTreeFile(t, filepath.Join(u.Dir, overridesLockFile), target+" "+sum+"\n")
			_, err = collectOverrides(root, []extensionUnit{u})
			if err == nil || !strings.Contains(err.Error(), "stays core's") {
				t.Fatalf("err = %v, want %s refused", err, target)
			}
		})
	}
}

func TestExtScreensGenRegistersEachAddedScreen(t *testing.T) {
	got := string(extScreensGen(nil, nil, []addedScreen{
		{Unit: "acme", Target: "src/custom/desk/screen.tsx", Source: "extensions/acme/frontend/overrides/src/custom/desk/screen.tsx"},
	}))
	for _, want := range []string{
		`import * as customScreen0 from "../../../extensions/acme/frontend/overrides/src/custom/desk/screen";`,
		"export const extensionCustomScreens = {\n  \"extensions/acme/frontend/overrides/src/custom/desk/screen.tsx\": customScreen0,\n};\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("emitted registry is missing %q:\n%s", want, got)
		}
	}
	if !strings.HasSuffix(string(extScreensGen(nil, nil, nil)), "export const extensionCustomScreens = {};\n") {
		t.Error("the empty set must still export an empty custom screen registry")
	}
}

func TestEmitOverridesWritesTheBundlerMapAndTheExportCheck(t *testing.T) {
	root, u := overrideTree(t)
	got, err := collectOverrides(root, []extensionUnit{u})
	if err != nil {
		t.Fatal(err)
	}
	if err := emitOverrides(root, got.Replaced); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, composedOverridesFile))
	if err != nil {
		t.Fatal(err)
	}
	var written []fileOverride
	if err := json.Unmarshal(raw, &written); err != nil {
		t.Fatal(err)
	}
	if len(written) != 1 || written[0] != got.Replaced[0] {
		t.Fatalf("bundler map = %+v, want %+v", written, got.Replaced)
	}
	check, err := os.ReadFile(filepath.Join(root, composedOverridesCheck))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`import type * as Original0 from "@margince-core/src/app/shellrail";`,
		"export type Check0 = Keeps<typeof Override0, typeof Original0>;",
	} {
		if !strings.Contains(string(check), want) {
			t.Errorf("type check is missing %q:\n%s", want, check)
		}
	}
}

// The frontend reads what emitOverrides writes, at paths it spells on its own
// side; this holds the two spellings together.
func TestFrontendReadsTheOverridesFilesWhereTheyAreWritten(t *testing.T) {
	for file, want := range map[string]string{
		"frontend/vite.config.ts":         composedOverridesFile,
		"frontend/tsconfig.composed.json": composedOverridesCheck,
	} {
		raw, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(file)))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), want) {
			t.Errorf("%s does not name %s, which gen-composition writes", file, want)
		}
	}
}

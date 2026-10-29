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

// overrideTree is a core with one menu file and a unit "acme" that replaces it,
// locked with lockLine.
func overrideTree(t *testing.T, lockLine string) (string, extensionUnit) {
	t.Helper()
	root := t.TempDir()
	writeTreeFile(t, filepath.Join(root, "frontend", railTarget), coreRail)
	u := overrideUnit(t, root, "acme")
	writeTreeFile(t, filepath.Join(u.Dir, overridesLayer, railTarget), "export * from \"@core-original/src/app/shellrail\";\n")
	writeTreeFile(t, filepath.Join(u.Dir, overridesLockFile), lockLine+"\n")
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
	root, u := overrideTree(t, "")
	writeTreeFile(t, filepath.Join(u.Dir, overridesLockFile), lockedRail(t, root)+"\n")

	got, err := collectOverrides(root, []extensionUnit{u})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Replaced) != 1 || got.Replaced[0].Target != railTarget || got.Replaced[0].Unit != "acme" {
		t.Fatalf("got %+v, want acme replacing %s", got.Replaced, railTarget)
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
		"no lock line": {
			edit: func(_ *testing.T, _ string, u extensionUnit) []extensionUnit { return []extensionUnit{u} },
			want: "names no hash for " + railTarget,
		},
		"core file changed since the lock": {
			edit: func(t *testing.T, root string, u extensionUnit) []extensionUnit {
				writeTreeFile(t, filepath.Join(u.Dir, overridesLockFile), lockedRail(t, root)+"\n")
				writeTreeFile(t, filepath.Join(root, "frontend", railTarget), coreRail+"// changed\n")
				return []extensionUnit{u}
			},
			want: "changed since the override was written",
		},
		"a new file that is not a custom screen": {
			edit: func(t *testing.T, root string, u extensionUnit) []extensionUnit {
				writeTreeFile(t, filepath.Join(u.Dir, overridesLockFile), lockedRail(t, root)+"\n")
				writeTreeFile(t, filepath.Join(u.Dir, overridesLayer, "src/app/gone.tsx"), "")
				return []extensionUnit{u}
			},
			want: "which this core does not have",
		},
		"a custom folder outside the name grammar": {
			edit: func(t *testing.T, root string, u extensionUnit) []extensionUnit {
				writeTreeFile(t, filepath.Join(u.Dir, overridesLockFile), lockedRail(t, root)+"\n")
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
				writeTreeFile(t, filepath.Join(u.Dir, overridesLockFile), line)
				other := overrideUnit(t, root, "beta")
				writeTreeFile(t, filepath.Join(other.Dir, overridesLayer, railTarget), "")
				writeTreeFile(t, filepath.Join(other.Dir, overridesLockFile), line)
				return []extensionUnit{u, other}
			},
			want: "acme and beta both override frontend/" + railTarget,
		},
		"two units, one added screen": {
			edit: func(t *testing.T, root string, u extensionUnit) []extensionUnit {
				writeTreeFile(t, filepath.Join(u.Dir, overridesLockFile), lockedRail(t, root)+"\n")
				writeTreeFile(t, filepath.Join(u.Dir, overridesLayer, "src/custom/desk/screen.tsx"), "")
				other := overrideUnit(t, root, "beta")
				writeTreeFile(t, filepath.Join(other.Dir, overridesLayer, "src/custom/desk/screen.tsx"), "")
				return []extensionUnit{u, other}
			},
			want: "acme and beta both override frontend/src/custom/desk/screen.tsx",
		},
		"lock pins a file the unit does not replace": {
			edit: func(t *testing.T, root string, u extensionUnit) []extensionUnit {
				writeTreeFile(t, filepath.Join(u.Dir, overridesLockFile), lockedRail(t, root)+"\nsrc/app/old.tsx sha256:00\n")
				return []extensionUnit{u}
			},
			want: "pins src/app/old.tsx",
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
			root, u := overrideTree(t, "")
			_, err := collectOverrides(root, tc.edit(t, root, u))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

// The screen registry imports an added screen by a path relative to where it
// is written, and keys it by its file so a key clash names the unit.
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
	root, u := overrideTree(t, "")
	writeTreeFile(t, filepath.Join(u.Dir, overridesLockFile), lockedRail(t, root)+"\n")
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
		`import type * as Original0 from "@core-original/src/app/shellrail";`,
		"export type Check0 = Keeps<typeof Override0, typeof Original0>;",
	} {
		if !strings.Contains(string(check), want) {
			t.Errorf("type check is missing %q:\n%s", want, check)
		}
	}
}

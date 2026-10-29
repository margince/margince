// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	dealsPkg        = "internal/modules/deals"
	createDealID    = dealsPkg + ".(*Store).CreateDeal"
	coreDealsSource = `package deals

type Store struct{}

// CreateDeal is core's.
func (s *Store) CreateDeal(name string) string { return "core:" + name }

func helper() int { return 1 }
`
	overlaySource = "// SPDX-License-Identifier: BUSL-1.1\n// SPDX-FileCopyrightText: 2026 Gradion\n\n" + overlayBuildTag + `

package deals

//margince:decorate
func (s *Store) CreateDeal(name string) string { return "acme:" + s.CreateDeal__core(name) }
`
)

// overlayTree is a core with one deals package and a unit "acme" decorating
// its CreateDeal, locked to core's current source.
func overlayTree(t *testing.T) (string, extensionUnit) {
	t.Helper()
	root := t.TempDir()
	writeTreeFile(t, filepath.Join(root, "backend/go.mod"), "module example.com/core\n\ngo 1.24\n")
	writeTreeFile(t, filepath.Join(root, "backend", dealsPkg, "deal.go"), coreDealsSource)
	u := extensionUnit{Name: "acme", Dir: filepath.Join(root, "extensions", "acme")}
	writeTreeFile(t, filepath.Join(u.Dir, overlaysLayer, dealsPkg, "acme.go"), overlaySource)
	lockCreateDeal(t, root, u)
	return root, u
}

func lockCreateDeal(t *testing.T, root string, u extensionUnit) {
	t.Helper()
	_, src, err := findCoreFunc(filepath.Join(root, "backend"), dealsPkg, "(*Store).CreateDeal")
	if err != nil {
		t.Fatal(err)
	}
	writeTreeFile(t, filepath.Join(u.Dir, overlaysLockFile), createDealID+" "+bytesSHA256(src)+"\n")
}

func readOverlayMap(t *testing.T, root string) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, composedOverlayFile))
	if err != nil {
		t.Fatal(err)
	}
	var overlay struct{ Replace map[string]string }
	if err := json.Unmarshal(raw, &overlay); err != nil {
		t.Fatal(err)
	}
	return overlay.Replace
}

func TestAnOverlayRenamesOnlyTheFunctionItDecorates(t *testing.T) {
	root, u := overlayTree(t)
	warnings, err := composeOverlays(root, []extensionUnit{u})
	if err != nil || len(warnings) != 0 {
		t.Fatalf("err = %v, warnings = %q; want neither", err, warnings)
	}
	replace := readOverlayMap(t, root)
	backend, err := filepath.EvalSymlinks(filepath.Join(root, "backend"))
	if err != nil {
		t.Fatal(err)
	}
	rewritten, err := os.ReadFile(replace[filepath.Join(backend, dealsPkg, "deal.go")])
	if err != nil {
		t.Fatalf("core file not in the overlay: %v", err)
	}
	if !strings.Contains(string(rewritten), "func (s *Store) CreateDeal__core(") || !strings.Contains(string(rewritten), "func helper() int") {
		t.Fatalf("rewritten core file:\n%s\nwant CreateDeal renamed and every other line kept", rewritten)
	}
	if len(replace) != 2 {
		t.Fatalf("overlay = %v, want the rewritten core file and the unit's file", replace)
	}
}

// The overlay go reads is the one a build uses: the decorated call runs the
// unit's function, which still reaches core's.
func TestGoBuildsTheOverlaidPackage(t *testing.T) {
	root, u := overlayTree(t)
	writeTreeFile(t, filepath.Join(root, "backend", dealsPkg, "deal_test.go"), `package deals

import "testing"

func TestDecorated(t *testing.T) {
	if got := (&Store{}).CreateDeal("x"); got != "acme:core:x" {
		t.Fatalf("CreateDeal = %q", got)
	}
}
`)
	if _, err := composeOverlays(root, []extensionUnit{u}); err != nil {
		t.Fatal(err)
	}
	if err := gatekit.DeclareInputs(os.Getenv, root); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "test", "-overlay="+filepath.Join(root, composedOverlayFile), "./"+dealsPkg)
	cmd.Dir = filepath.Join(root, "backend")
	cmd.Env = append(os.Environ(), "GOWORK=off", "GOFLAGS=")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go test with the overlay: %v\n%s", err, out)
	}
}

func TestNoOverlaysStillWritesAnEmptyMap(t *testing.T) {
	root := t.TempDir()
	writeTreeFile(t, filepath.Join(root, "backend/go.mod"), "module example.com/core\n")
	if _, err := composeOverlays(root, nil); err != nil {
		t.Fatal(err)
	}
	if replace := readOverlayMap(t, root); len(replace) != 0 {
		t.Fatalf("overlay = %v, want an empty map", replace)
	}
}

func TestLockDriftOnAnOverlayWarns(t *testing.T) {
	cases := map[string]struct {
		edit func(t *testing.T, root string, u extensionUnit)
		want string
	}{
		"no lock line": {
			edit: func(t *testing.T, _ string, u extensionUnit) {
				writeTreeFile(t, filepath.Join(u.Dir, overlaysLockFile), "")
			},
			want: "names no hash for " + createDealID,
		},
		"core changed the function": {
			edit: func(t *testing.T, root string, _ extensionUnit) {
				writeTreeFile(t, filepath.Join(root, "backend", dealsPkg, "deal.go"), strings.Replace(coreDealsSource, `"core:"`, `"core2:"`, 1))
			},
			want: "changed since the overlay was written",
		},
		"a line no overlay uses": {
			edit: func(t *testing.T, root string, u extensionUnit) {
				lockCreateDeal(t, root, u)
				f, err := os.OpenFile(filepath.Join(u.Dir, overlaysLockFile), os.O_APPEND|os.O_WRONLY, 0)
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					if err := f.Close(); err != nil {
						t.Error(err)
					}
				}()
				if _, err := f.WriteString(dealsPkg + ".gone sha256:00\n"); err != nil {
					t.Fatal(err)
				}
			},
			want: "pins " + dealsPkg + ".gone",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			root, u := overlayTree(t)
			tc.edit(t, root, u)
			warnings, err := composeOverlays(root, []extensionUnit{u})
			if err != nil {
				t.Fatal(err)
			}
			if len(warnings) != 1 || !strings.Contains(warnings[0], tc.want) {
				t.Fatalf("warnings = %q, want one containing %q", warnings, tc.want)
			}
			if len(readOverlayMap(t, root)) != 2 {
				t.Fatal("a drifted lock must still apply the overlay")
			}
		})
	}
}

func TestOverlayRefusals(t *testing.T) {
	cases := map[string]struct {
		edit func(t *testing.T, root string, u extensionUnit) []extensionUnit
		want string
	}{
		"no build tag": {
			edit: func(t *testing.T, _ string, u extensionUnit) []extensionUnit {
				writeTreeFile(t, filepath.Join(u.Dir, overlaysLayer, dealsPkg, "acme.go"), strings.Replace(overlaySource, overlayBuildTag, "", 1))
				return []extensionUnit{u}
			},
			want: "must carry",
		},
		"two decorators of one function": {
			edit: func(t *testing.T, root string, u extensionUnit) []extensionUnit {
				other := extensionUnit{Name: "beta", Dir: filepath.Join(root, "extensions", "beta")}
				writeTreeFile(t, filepath.Join(other.Dir, overlaysLayer, dealsPkg, "beta.go"), overlaySource)
				return []extensionUnit{u, other}
			},
			want: "acme and beta both decorate " + createDealID,
		},
		"a package outside the modules": {
			edit: func(t *testing.T, root string, u extensionUnit) []extensionUnit {
				writeTreeFile(t, filepath.Join(root, "backend/internal/platform/auth/auth.go"), "package auth\n")
				writeTreeFile(t, filepath.Join(u.Dir, overlaysLayer, "internal/platform/auth/acme.go"), overlayBuildTag+"\n\npackage auth\n")
				return []extensionUnit{u}
			},
			want: "only packages under backend/internal/modules/",
		},
		"a package core does not have": {
			edit: func(t *testing.T, _ string, u extensionUnit) []extensionUnit {
				writeTreeFile(t, filepath.Join(u.Dir, overlaysLayer, "internal/modules/gone/acme.go"), overlayBuildTag+"\n\npackage gone\n")
				return []extensionUnit{u}
			},
			want: "which this core does not have",
		},
		"code that runs at load": {
			edit: func(t *testing.T, _ string, u extensionUnit) []extensionUnit {
				writeTreeFile(t, filepath.Join(u.Dir, overlaysLayer, dealsPkg, "acme.go"), overlaySource+"\nfunc init() {}\n")
				return []extensionUnit{u}
			},
			want: "declares init",
		},
		"a package-level var": {
			edit: func(t *testing.T, _ string, u extensionUnit) []extensionUnit {
				writeTreeFile(t, filepath.Join(u.Dir, overlaysLayer, dealsPkg, "acme.go"), overlaySource+"\nvar hooked = helper()\n")
				return []extensionUnit{u}
			},
			want: "declares a package-level var",
		},
		"a linkname directive": {
			edit: func(t *testing.T, _ string, u extensionUnit) []extensionUnit {
				writeTreeFile(t, filepath.Join(u.Dir, overlaysLayer, dealsPkg, "acme.go"), overlaySource+"\n//go:linkname reach other.secret\nfunc reach() int\n")
				return []extensionUnit{u}
			},
			want: "carries //go:linkname",
		},
		"unsafe": {
			edit: func(t *testing.T, _ string, u extensionUnit) []extensionUnit {
				writeTreeFile(t, filepath.Join(u.Dir, overlaysLayer, dealsPkg, "acme.go"), strings.Replace(overlaySource, "package deals\n", "package deals\n\nimport _ \"unsafe\"\n", 1))
				return []extensionUnit{u}
			},
			want: "imports unsafe",
		},
		"a core package the target does not import": {
			edit: func(t *testing.T, _ string, u extensionUnit) []extensionUnit {
				writeTreeFile(t, filepath.Join(u.Dir, overlaysLayer, dealsPkg, "acme.go"), strings.Replace(overlaySource, "package deals\n", "package deals\n\nimport _ \"example.com/core/internal/platform/auth\"\n", 1))
				return []extensionUnit{u}
			},
			want: "imports example.com/core/internal/platform/auth",
		},
		"a function core does not have": {
			edit: func(t *testing.T, _ string, u extensionUnit) []extensionUnit {
				writeTreeFile(t, filepath.Join(u.Dir, overlaysLayer, dealsPkg, "acme.go"), strings.ReplaceAll(overlaySource, "CreateDeal", "CreateDael"))
				return []extensionUnit{u}
			},
			want: "decorates " + dealsPkg + ".(*Store).CreateDael, which this core does not have",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			root, u := overlayTree(t)
			_, err := composeOverlays(root, tc.edit(t, root, u))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want one containing %q", err, tc.want)
			}
		})
	}
}

func TestTheManifestNamesEachDecoratedFunction(t *testing.T) {
	_, u := overlayTree(t)
	got, err := declaredOverlays(u.Dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != createDealID {
		t.Fatalf("declared = %q, want [%s]", got, createDealID)
	}
}

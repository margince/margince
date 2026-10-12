// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H2

package gates

// A file behind the bench build tag says so in its name, so
// `git ls-files '*_bench_test.go'` lists every bench in the tree.
//
// The tag decides what runs; the name is how a reader finds it. A bench file
// named like an ordinary integration test is invisible to that listing while
// still being one of the benches, so the census a developer runs by hand reads
// short and nothing fails.
//
// What this does not catch: a bench outside backend/. The walk stays in this
// module because the other roots are separate modules the test cache cannot
// see, and every bench file lives here today.

import (
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	benchTag        = "bench"
	benchFileSuffix = "_bench_test.go"
	// benchFileFloor sits below the count this tree carries, so it catches a
	// walk that stopped reading files rather than a bench being retired.
	benchFileFloor = 5
)

func TestEveryBenchFileSaysSoInItsName(t *testing.T) {
	t.Parallel()
	var found int
	var misnamed []string
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") {
			return walkErr
		}
		file, err := gatekit.ParseFile(path, parser.PackageClauseOnly|parser.ParseComments)
		if err != nil {
			return err
		}
		bench, refused, err := judgeBenchFile(path, file)
		if err != nil {
			return err
		}
		if bench {
			found++
		}
		if refused {
			misnamed = append(misnamed, filepath.ToSlash(path))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the module for bench-tagged files: %v", err)
	}
	if found < benchFileFloor {
		t.Errorf("the walk found %d bench-tagged files; this tree carries at least %d, so the scan is "+
			"reading a smaller tree than exists (a walk that stopped early, or a constraint reader that "+
			"no longer recognises the tag) rather than proving the names", found, benchFileFloor)
	}
	for _, path := range misnamed {
		t.Errorf("%s carries the %q build tag but its name does not end in %s, so "+
			"`git ls-files '*%s'` leaves it out: rename it with git mv to <suite>_<part>%s",
			path, benchTag, benchFileSuffix, benchFileSuffix, benchFileSuffix)
	}
}

// judgeBenchFile reports whether the file at path is bench-tagged, and whether
// its name hides that from a listing by suffix.
func judgeBenchFile(path string, file *ast.File) (bench, refused bool, err error) {
	bench, err = isBenchTagged(file)
	return bench, bench && !strings.HasSuffix(path, benchFileSuffix), err
}

// isBenchTagged reports whether the file's build constraint asks for the bench
// tag. Only constraint lines above the package clause count, which is where the
// toolchain reads them.
func isBenchTagged(file *ast.File) (bool, error) {
	for _, group := range file.Comments {
		if group.Pos() >= file.Package {
			break
		}
		for _, line := range group.List {
			if !constraint.IsGoBuild(line.Text) && !constraint.IsPlusBuild(line.Text) {
				continue
			}
			expr, err := constraint.Parse(line.Text)
			if err != nil {
				return false, err
			}
			if requiresTag(expr, benchTag, true) {
				return true, nil
			}
		}
	}
	return false, nil
}

// requiresTag reports whether tag appears in expr under an even number of
// negations: `!bench` excludes the bench lane rather than joining it.
func requiresTag(expr constraint.Expr, tag string, positive bool) bool {
	switch node := expr.(type) {
	case *constraint.TagExpr:
		return positive && node.Tag == tag
	case *constraint.NotExpr:
		return requiresTag(node.X, tag, !positive)
	case *constraint.AndExpr:
		return requiresTag(node.X, tag, positive) || requiresTag(node.Y, tag, positive)
	case *constraint.OrExpr:
		return requiresTag(node.X, tag, positive) || requiresTag(node.Y, tag, positive)
	}
	return false
}

func TestBenchFileNameGateRefusesAMisnamedBench(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, constraint       string
		wantBench, wantRefused bool
	}{
		{"x_test.go", "//go:build integration && bench", true, true},
		{"x_bench_test.go", "//go:build integration && bench", true, false},
		{"x_test.go", "//go:build !bench", false, false},
		{"x_test.go", "//go:build integration && !(!bench)", true, true},
		{"x_test.go", "// +build bench", true, true},
	}
	for _, tc := range cases {
		src := "// SPDX-License-Identifier: BUSL-1.1\n\n" + tc.constraint + "\n\npackage x\n"
		file, err := parser.ParseFile(token.NewFileSet(), tc.name, src, parser.PackageClauseOnly|parser.ParseComments)
		if err != nil {
			t.Fatalf("parsing the planted %s: %v", tc.name, err)
		}
		bench, refused, err := judgeBenchFile(tc.name, file)
		if err != nil {
			t.Fatalf("reading the planted constraint %q: %v", tc.constraint, err)
		}
		if bench != tc.wantBench || refused != tc.wantRefused {
			t.Errorf("%s with %q: bench=%v refused=%v, want bench=%v refused=%v",
				tc.name, tc.constraint, bench, refused, tc.wantBench, tc.wantRefused)
		}
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Removing a released registration must fail even when its ID constant remains.

import (
	"archive/tar"
	"bytes"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/datatask"
)

func sourceTaskIDs(source string) ([]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), "catalog.go", source, 0)
	if err != nil {
		return nil, err
	}
	found := map[string]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		literal, ok := node.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(literal.Value)
		if err == nil && datatask.ValidTaskID(value) {
			found[value] = true
		}
		return true
	})
	out := make([]string, 0, len(found))
	for id := range found {
		out = append(out, id)
	}
	slices.Sort(out)
	return out, nil
}

func removedTaskIDs(before, after []string) []string {
	current := map[string]bool{}
	for _, id := range after {
		current[id] = true
	}
	var removed []string
	for _, id := range before {
		if !current[id] {
			removed = append(removed, id)
		}
	}
	slices.Sort(removed)
	return slices.Compact(removed)
}

func TestReleasedDataTaskIDsStayInTheWorkerCatalog(t *testing.T) {
	t.Parallel()
	current := compose.DataTaskIDs()
	if len(current) == 0 {
		t.Fatal("empty worker task census")
	}
	declared, err := currentTaskLiterals()
	if err != nil {
		t.Fatal(err)
	}
	if missing := removedTaskIDs(current, declared); len(missing) > 0 {
		t.Fatalf("task source census misses worker IDs: %v; keep released IDs as static literals", missing)
	}
	released, err := releasedTaskLiterals()
	if err != nil {
		t.Fatal(err)
	}

	if missing := removedTaskIDs(released, current); len(missing) > 0 {
		t.Fatalf("released tasks removed from worker catalog: %v; task retirement requires a declared minimum upgrade-from release", missing)
	}
}

func TestTaskCensusFindsConstantsButDoesNotReadCommentsOrRequires(t *testing.T) {
	t.Parallel()
	source := `package compose
 // core/100_comment
 const held = "core/200_held"
 const unrelated = "core/0001_baseline.sql"
 var tasks = []Task{{ID:held, Requires:[]string{"core/150"}}, {ID:"core/300_next"}}
 `
	ids, err := sourceTaskIDs(source)
	if err != nil || !slices.Equal(ids, []string{"core/200_held", "core/300_next"}) {
		t.Fatalf("task census=%v error=%v", ids, err)
	}
	if removed := removedTaskIDs(ids, []string{"core/300_next"}); !slices.Equal(removed, []string{"core/200_held"}) {
		t.Fatalf("missed retired task: %v", removed)
	}
	if removed := removedTaskIDs(ids, append(ids, "core/400_new")); len(removed) != 0 {
		t.Fatalf("append-only task addition refused: %v", removed)
	}
	if _, err := sourceTaskIDs("not Go source"); err == nil {
		t.Fatal("invalid catalog source passed")
	}
}

func currentTaskLiterals() ([]string, error) {
	paths, err := filepath.Glob("internal/compose/*.go")
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, errors.New("empty compose source census")
	}
	var out []string
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		ids, err := sourceTaskIDs(string(raw))
		if err != nil {
			return nil, err
		}
		out = append(out, ids...)
	}
	return out, nil
}

func releasedTaskLiterals() ([]string, error) {
	body, err := exec.Command("git", "-C", repoRoot, "archive", "origin/main", "backend/internal/compose").Output()
	if err != nil {
		return nil, err
	}
	archive := tar.NewReader(bytes.NewReader(body))
	var out []string
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		if header.Typeflag != tar.TypeReg || !strings.HasSuffix(header.Name, ".go") || strings.HasSuffix(header.Name, "_test.go") {
			continue
		}
		source, err := io.ReadAll(archive)
		if err != nil {
			return nil, err
		}
		ids, err := sourceTaskIDs(string(source))
		if err != nil {
			return nil, err
		}
		out = append(out, ids...)
	}
}

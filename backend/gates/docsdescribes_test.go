// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H1

//go:build !integration

package gates

// Every explanation and handbook page names the code it describes.
//
// A pull request that changes that code is asked whether the page still holds
// (scripts/check-docs-describing.sh). A header naming a moved file asks nothing,
// so each path must exist. Whether the paths are the right ones is for review.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// describesHeader is the line a page carries before its title.
var describesHeader = regexp.MustCompile(`^<!-- describes: (\S+(?: \S+)*) -->$`)

// describingFolders are the folders whose pages tell what the product does.
// Reference pages are lists, most of them generated; how-tos are steps.
var describingFolders = []string{"docs/explanation", "docs/handbook"}

func TestEveryDescribingPageNamesTheCodeItDescribes(t *testing.T) {
	t.Parallel()
	pages := 0
	for _, folder := range describingFolders {
		matches, err := filepath.Glob(filepath.Join(docsTreeRoot, folder, "*.md"))
		if err != nil {
			t.Fatalf("glob %s: %v", folder, err)
		}
		for _, path := range matches {
			if filepath.Base(path) == "README.md" {
				continue
			}
			pages++
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v", path, err)
			}
			rel := filepath.ToSlash(strings.TrimPrefix(path, docsTreeRoot+string(filepath.Separator)))
			for _, problem := range describesProblems(string(raw), pathExistsInTree) {
				t.Errorf("%s: %s", rel, problem)
			}
		}
	}
	if pages == 0 {
		t.Fatalf("no pages under %v — an empty census passes like a correct one", describingFolders)
	}
}

// pathExistsInTree reports whether a repository-relative path is a file or a
// folder in this checkout.
func pathExistsInTree(rel string) bool {
	_, err := os.Stat(filepath.Join(docsTreeRoot, filepath.FromSlash(rel)))
	return err == nil
}

// describesProblems judges one page's header. The header sits above the title,
// so a reader of the raw page meets it first and the renderer shows nothing.
func describesProblems(page string, exists func(string) bool) []string {
	var headers [][]string
	for _, line := range strings.Split(page, "\n") {
		if strings.HasPrefix(line, "# ") {
			break
		}
		if match := describesHeader.FindStringSubmatch(line); match != nil {
			headers = append(headers, strings.Fields(match[1]))
		}
	}
	switch len(headers) {
	case 0:
		return []string{"no `<!-- describes: <path> ... -->` line above the title. Name the code whose " +
			"behaviour this page states, so a pull request that changes it is asked whether the page still holds"}
	case 1:
	default:
		return []string{"more than one describes line; keep one"}
	}
	var problems []string
	seen := map[string]bool{}
	for _, path := range headers[0] {
		switch {
		case seen[path]:
			problems = append(problems, "names "+path+" twice")
		case strings.HasPrefix(path, "/") || strings.Contains(path, ".."):
			problems = append(problems, path+" is not a path from the repository root")
		case !exists(strings.TrimSuffix(path, "/")):
			problems = append(problems, path+" does not exist. Name the file or folder that holds this code now")
		}
		seen[path] = true
	}
	return problems
}

func TestTheDescribesHeaderIsReadAboveTheTitleOnly(t *testing.T) {
	t.Parallel()
	exists := func(rel string) bool { return rel == "backend/internal/modules/comms" || rel == "frontend/src/app.tsx" }
	tests := []struct {
		name     string
		page     string
		problems int
	}{
		{"one header naming real paths", "<!-- prose:plain -->\n<!-- describes: backend/internal/modules/comms/ frontend/src/app.tsx -->\n# T\n", 0},
		{"no header", "<!-- prose:plain -->\n# T\n", 1},
		{"a header under the title is prose", "# T\n<!-- describes: frontend/src/app.tsx -->\n", 1},
		{"two headers", "<!-- describes: frontend/src/app.tsx -->\n<!-- describes: frontend/src/app.tsx -->\n# T\n", 1},
		{"a moved file", "<!-- describes: frontend/src/gone.tsx -->\n# T\n", 1},
		{"a path out of the tree", "<!-- describes: ../outside -->\n# T\n", 1},
		{"the same path twice", "<!-- describes: frontend/src/app.tsx frontend/src/app.tsx -->\n# T\n", 1},
	}
	for _, tc := range tests {
		if got := describesProblems(tc.page, exists); len(got) != tc.problems {
			t.Errorf("%s: got %d problems %v, want %d", tc.name, len(got), got, tc.problems)
		}
	}
}

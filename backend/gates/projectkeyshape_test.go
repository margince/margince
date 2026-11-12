// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H1

package gates

// A project key has one shape in Go, projectkey.Shaped. The projects module
// mints and refuses keys with it, and capture reads subject lines with it.
// A second spelling, a test's included, is an answer that can drift.

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	projectKeyShapeOwner = "internal/shared/kernel/projectkey/projectkey.go"
	// projectKeyShapeBody is the part of the pattern no other shape shares.
	projectKeyShapeBody = "[A-Za-z][A-Za-z0-9_-]"
)

func TestTheProjectKeyShapeIsSpelledOnce(t *testing.T) {
	t.Parallel()
	var owners, others []string
	files := 0
	err := gatekit.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		files++
		body, err := gatekit.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.Contains(string(body), projectKeyShapeBody) {
			return nil
		}
		path = filepath.ToSlash(path)
		switch path {
		case projectKeyShapeOwner:
			owners = append(owners, path)
		case "gates/projectkeyshape_test.go":
		default:
			others = append(others, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the source tree: %v", err)
	}
	if files < 1000 || len(owners) != 1 {
		t.Fatalf("read %d Go files and found the owner %d times; the census went short", files, len(owners))
	}
	for _, path := range others {
		t.Errorf("%s spells the project key shape again; use projectkey.Shaped or projectkey.CheckPattern", path)
	}
}

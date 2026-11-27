// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/text/unicode/norm"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// Every string literal the product ships is NFC.
//
// A decomposed "xóa" renders the same as a composed one, and an equality, a
// map key or a search against the composed text misses it. The shared copy
// packages import only the standard library, so this x/text check runs here.
func TestEveryShippedStringLiteralIsNFC(t *testing.T) {
	t.Parallel()
	files, literals := 0, 0
	for _, top := range []string{"backend", "extensions"} {
		err := gatekit.WalkDir(filepath.Join(repoRoot, top), func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil || !shippedGoFile(entry, path) {
				return walkErr
			}
			file, err := gatekit.ParseFile(path, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			files++
			read, decomposed := notNFC(file)
			literals += read
			for _, lit := range decomposed {
				t.Errorf("%s: %s is not NFC; write it composed (norm.NFC.String gives the spelling)",
					gatekit.SourceFileSet().Position(lit.Pos()), lit.Value)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if files < 5000 || literals < 150000 {
		t.Fatalf("read %d Go files and %d string literals; the walk is reading another tree", files, literals)
	}
}

func TestADecomposedLiteralIsNotNFC(t *testing.T) {
	t.Parallel()
	source := "package p\n\nconst (\n\tcomposed = \"Đã xóa\"\n\tdecomposed = \"" + norm.NFD.String("Đã xóa") + "\"\n" +
		"\traw = `" + norm.NFD.String("Kính gửi") + "`\n\tascii = \"Deleted\"\n)\n"
	file, err := parser.ParseFile(token.NewFileSet(), "planted.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	read, decomposed := notNFC(file)
	if read != 4 || len(decomposed) != 2 {
		t.Errorf("read %d literals and found %d not NFC, want 4 and 2", read, len(decomposed))
	}
}

// notNFC answers how many string literals a file holds and those that are not
// NFC.
func notNFC(file *ast.File) (int, []*ast.BasicLit) {
	read := 0
	var decomposed []*ast.BasicLit
	ast.Inspect(file, func(node ast.Node) bool {
		lit, isLit := node.(*ast.BasicLit)
		if !isLit {
			return true
		}
		if text, isString := gatekit.StringExpr(lit, nil, gatekit.FoldStrict); isString {
			read++
			if !norm.NFC.IsNormalString(text) {
				decomposed = append(decomposed, lit)
			}
		}
		return true
	})
	return read, decomposed
}

// shippedGoFile is a Go file that builds into a program: not a test, and not
// under testdata or a hidden directory.
func shippedGoFile(entry fs.DirEntry, path string) bool {
	if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
		return false
	}
	for segment := range strings.SplitSeq(filepath.ToSlash(path), "/") {
		if segment == "testdata" || segment == "node_modules" || (len(segment) > 1 && segment[0] == '.' && segment != "..") {
			return false
		}
	}
	return true
}

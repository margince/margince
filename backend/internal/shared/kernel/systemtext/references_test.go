// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemtext

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func TestBuiltInMessageReferencesResolve(t *testing.T) {
	root := gatekit.ModuleRoot(t)
	references := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		codes, err := referencedCodes(path, source)
		if err != nil {
			return err
		}
		for _, code := range codes {
			references++
			if _, ok := definitionOf(code); !ok {
				t.Errorf("%s references unregistered message %s", path, code)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if references == 0 {
		t.Fatal("no message references were read")
	}
}

func referencedCodes(path string, source []byte) ([]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, source, 0)
	if err != nil {
		return nil, err
	}
	aliases := map[string]bool{}
	for _, spec := range file.Imports {
		imported, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return nil, err
		}
		if !strings.HasSuffix(imported, "/internal/shared/kernel/systemtext") {
			continue
		}
		name := "systemtext"
		if spec.Name != nil {
			name = spec.Name.Name
		}
		if name == "." {
			return nil, fmt.Errorf("%s uses a dot import for system messages", path)
		}
		aliases[name] = true
	}
	var codes []string
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		alias, ok := selector.X.(*ast.Ident)
		if !ok || !aliases[alias.Name] {
			return true
		}
		switch selector.Sel.Name {
		case "Sentence", "Literal", "Copy", "BoundLiteral", "Reference", "LocalizedLiteral":
		default:
			return true
		}
		literal, ok := call.Args[0].(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		code, err := strconv.Unquote(literal.Value)
		if err == nil {
			codes = append(codes, code)
		}
		return true
	})
	return codes, nil
}

func TestReferenceCensusFollowsCallsInsideHelpers(t *testing.T) {
	source := []byte(`package producer
 import text "github.com/margince/margince/backend/internal/shared/kernel/systemtext"
 func hidden() { text.Sentence("missing");text.Copy("core.erased","") }
 `)
	codes, err := referencedCodes("producer.go", source)
	if err != nil || len(codes) != 2 || codes[0] != "missing" {
		t.Fatalf("helper references were missed: %v %v", codes, err)
	}
	if _, ok := definitionOf(codes[0]); ok {
		t.Fatal("unregistered code was accepted")
	}
}

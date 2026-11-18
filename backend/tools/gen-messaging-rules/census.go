// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/platform/messagingpolicy"
)

const (
	// generatorName is how a unit's //go:generate line names this tool.
	generatorName = "gen-messaging-rules"
	// rulesFunc is the function the generated file declares and New calls.
	rulesFunc = "messagingRules"
)

// unitsFrom answers the extension folders to compile. With -root a unit is any
// folder whose hand-written Go calls messagingRules, or that holds a policy
// folder or a generated file. The call outlives both files, so a unit whose
// policy and generated file were deleted together is still found.
func unitsFrom(root, dir string) ([]string, error) {
	if (root == "") == (dir == "") {
		return nil, errors.New("gen-messaging-rules: pass exactly one of -root and -dir")
	}
	if dir != "" {
		return []string{dir}, nil
	}
	folders, err := filepath.Glob(filepath.Join(root, "extensions", "*"))
	if err != nil {
		return nil, err
	}
	var units []string
	for _, folder := range folders {
		if info, err := os.Stat(folder); err != nil || !info.IsDir() {
			continue
		}
		isUnit, err := isRulesUnit(folder)
		if err != nil {
			return nil, err
		}
		if isUnit {
			units = append(units, folder)
		}
	}
	if len(units) == 0 {
		return nil, fmt.Errorf("gen-messaging-rules: no extension under %s declares messaging rules; check -root", root)
	}
	return units, nil
}

func isRulesUnit(folder string) (bool, error) {
	for _, own := range []string{"policy", messagingpolicy.GeneratedFile} {
		if _, err := os.Stat(filepath.Join(folder, own)); err == nil {
			return true, nil
		}
	}
	files, err := handWritten(folder)
	if err != nil {
		return false, err
	}
	return callsRulesFunc(files), nil
}

// handWritten parses the unit's Go files that no tool wrote and no test owns.
func handWritten(unit string) ([]*ast.File, error) {
	paths, err := filepath.Glob(filepath.Join(unit, "*.go"))
	if err != nil {
		return nil, err
	}
	var out []*ast.File
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		out = append(out, file)
	}
	return out, nil
}

func callsRulesFunc(files []*ast.File) bool {
	return slices.ContainsFunc(files, func(file *ast.File) bool {
		found := false
		ast.Inspect(file, func(n ast.Node) bool {
			if call, isCall := n.(*ast.CallExpr); isCall {
				if ident, isIdent := call.Fun.(*ast.Ident); isIdent && ident.Name == rulesFunc {
					found = true
				}
			}
			return !found
		})
		return found
	})
}

// generatesRules reports whether a //go:generate line in files runs this tool.
func generatesRules(files []*ast.File) bool {
	return slices.ContainsFunc(files, func(file *ast.File) bool {
		return slices.ContainsFunc(file.Comments, func(group *ast.CommentGroup) bool {
			return slices.ContainsFunc(group.List, func(c *ast.Comment) bool {
				return strings.HasPrefix(c.Text, "//go:generate ") && strings.Contains(c.Text, generatorName)
			})
		})
	})
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// toolRun is a recipe line that runs a generator of the tools module.
var toolRun = regexp.MustCompile(`\./tools/([\w-]+)`)

// recipeTools names the tools the recipes of targets run.
func recipeTools(makefile string, targets []string) []string {
	var tools []string
	inTarget := false
	for line := range strings.SplitSeq(makefile, "\n") {
		if !strings.HasPrefix(line, "\t") {
			name, _, isRule := strings.Cut(line, ":")
			inTarget = isRule && slices.Contains(targets, name)
			continue
		}
		if inTarget {
			for _, match := range toolRun.FindAllStringSubmatch(line, -1) {
				tools = appendNew(tools, match[1])
			}
		}
	}
	slices.Sort(tools)
	return tools
}

// namingTools maps each path a tool's source names, as a string literal, to
// the tool. Some tools write a file only when its bytes change, and some hold a
// committed copy to their own output. Such a tool names the file and leaves
// its time alone.
func namingTools(root string, tools []string) (map[string]string, error) {
	named := map[string]string{}
	fset := token.NewFileSet()
	for _, tool := range tools {
		dir := filepath.Join(root, "backend", "tools", tool)
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			name := entry.Name()
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
			if err != nil {
				return nil, err
			}
			ast.Inspect(file, func(node ast.Node) bool {
				if lit, ok := node.(*ast.BasicLit); ok && lit.Kind == token.STRING {
					if value, err := strconv.Unquote(lit.Value); err == nil && value != "" {
						named[value] = tool
					}
				}
				return true
			})
		}
	}
	return named, nil
}

// namedBy answers the tool that names output by its path, a trailing part of
// it, or its file name.
func namedBy(output string, named map[string]string) (string, bool) {
	parts := strings.Split(output, "/")
	for i := range parts {
		if tool, ok := named[strings.Join(parts[i:], "/")]; ok {
			return tool, true
		}
	}
	return "", false
}

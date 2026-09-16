// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestRecoveryBindingsNameFieldsOfTheRegisteredJobArgs(t *testing.T) {
	registry, _ := wireJobs(nil, slog.New(slog.DiscardHandler), JobRunnerConfig{})
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), entry.Name(), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			method, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || method.Sel.Name != "ResumeScheduledTx" {
				return true
			}
			if len(call.Args) != 6 {
				t.Fatal("recovery call shape changed; inspect its binding")
			}
			literal := func(expr ast.Expr) string {
				value, ok := expr.(*ast.BasicLit)
				if !ok || value.Kind != token.STRING {
					t.Fatal("recovery binding is not a declared literal")
				}
				decoded, err := strconv.Unquote(value.Value)
				if err != nil {
					t.Fatal(err)
				}
				return decoded
			}
			kind, field := literal(call.Args[2]), literal(call.Args[3])
			worker, ok := registry.wired[kind]
			if !ok {
				t.Fatalf("recovery names unregistered job %q", kind)
			}
			raw, err := json.Marshal(worker.args)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(raw, &fields); err != nil {
				t.Fatal(err)
			}
			if _, ok := fields[field]; !ok {
				t.Fatalf("%s recovery names absent argument %s", kind, field)
			}
			if _, ok := fields["workspace_id"]; !ok {
				t.Fatalf("%s recovery has no workspace argument", kind)
			}
			checked++
			return true
		})
	}
	if checked == 0 {
		t.Fatal("no recovery bindings were inspected")
	}
}

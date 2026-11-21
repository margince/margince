// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// stopWith is a stop that words its run with cause.
func stopWith(cause string) error {
	return RunStop{Cause: cause, Err: errors.New(cause)}
}

// A move that writes a new sentence clears the code of the one before it.
func TestAMoveThatWritesASentenceClearsAnOlderRefusal(t *testing.T) {
	t.Parallel()
	got := refusalInStep([]runValue{{errorColumn, CauseDraftExpired}})
	want := []runValue{{errorColumn, CauseDraftExpired}, {refusalColumn, nil}}
	if !slices.Equal(got, want) {
		t.Errorf("a new sentence writes %v, want %v", got, want)
	}
	named := []runValue{{errorColumn, "line 7"}, {refusalColumn, []byte(`{"field":"file"}`)}}
	if got := refusalInStep(named); len(got) != 2 || got[1].column != refusalColumn {
		t.Errorf("a sentence with its own refusal writes %v, want it kept", got)
	}
	other := []runValue{{"report", []byte(`{}`)}}
	if got := refusalInStep(other); len(got) != 1 {
		t.Errorf("a move that writes no sentence writes %v, want it unchanged", got)
	}
}

// Every statement of this package that assigns the sentence assigns its
// refusal too. The census counts the writers, so a scan that reads none fails.
func TestEveryWriteOfTheSentenceWritesItsRefusal(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	assigns := regexp.MustCompile(`(?:^|SET|,)\s*error\s*=`)
	writers := 0
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			literal, ok := node.(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING || !assigns.MatchString(literal.Value[1:]) {
				return true
			}
			writers++
			if !strings.Contains(literal.Value, refusalColumn) {
				t.Errorf("%s: %s assigns error and leaves %s as it was", name, literal.Value, refusalColumn)
			}
			return true
		})
	}
	if writers < 5 {
		t.Fatalf("read %d statements that assign error, want the package's writers", writers)
	}
}

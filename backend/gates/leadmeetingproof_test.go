// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

//go:build !integration

package gates

// Lead evidence, lead scoring and the lead status ladder read what a meeting
// proves from leadmeetingproof.go. Two readers with their own rule disagree on
// an unrecorded meeting, so this refuses a status comparison elsewhere in the
// module.
//
// Go comparisons are read through the AST and SQL as literal text. A status
// word held in a variable, or SQL split between literals, is not seen.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const (
	leadMeetingProofModule = "internal/modules/contacts"
	leadMeetingProofOwner  = "leadmeetingproof.go"
)

// leadMeetingStatusSQL is a comparison on the column, the NULL test included.
var leadMeetingStatusSQL = regexp.MustCompile(`meeting_status\s*(?:=|<>|!=|IN\b|IS\b|NOT\b)`)

// leadMeetingStatusWords are the statuses a Go reader would compare against.
var leadMeetingStatusWords = map[string]bool{"held": true, "booked": true, "canceled": true, "no_show": true}

func TestOnlyLeadMeetingProofJudgesAMeetingOnALead(t *testing.T) {
	t.Parallel()
	files, err := filepath.Glob(filepath.Join(leadMeetingProofModule, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	scanned, ownerFindings := 0, 0
	var strays []string
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		scanned++
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, at := range meetingStatusJudgements(fset, file) {
			if filepath.Base(path) == leadMeetingProofOwner {
				ownerFindings++
				continue
			}
			strays = append(strays, path+":"+at)
		}
	}
	if scanned < 300 {
		t.Fatalf("read %d Go files in %s and expected far more; the gate has stopped reaching its corpus", scanned, leadMeetingProofModule)
	}
	// The owner holds both forms, so a finder that went blind fails here.
	if ownerFindings < 2 {
		t.Fatalf("found %d meeting-status judgements in %s, want its Go and SQL forms; the finder no longer sees its subject", ownerFindings, leadMeetingProofOwner)
	}
	if len(strays) > 0 {
		t.Errorf("a meeting's status is judged outside %s:\n\t%s\nRead meetingProof, meetingProves or meetingProofSQL instead, "+
			"so lead evidence, scoring and the ladder keep one answer.", leadMeetingProofOwner, strings.Join(strays, "\n\t"))
	}
}

// meetingStatusJudgements answers the line of each status comparison in file.
func meetingStatusJudgements(fset *token.FileSet, file *ast.File) []string {
	var out []string
	mark := func(n ast.Node) { out = append(out, strconv.Itoa(fset.Position(n.Pos()).Line)) }
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.BinaryExpr:
			if (node.Op == token.EQL || node.Op == token.NEQ) && (isStatusWord(node.X) || isStatusWord(node.Y)) {
				mark(node)
			}
		case *ast.CaseClause:
			for _, expr := range node.List {
				if isStatusWord(expr) {
					mark(expr)
				}
			}
		case *ast.BasicLit:
			if node.Kind == token.STRING && leadMeetingStatusSQL.MatchString(node.Value) {
				mark(node)
			}
		}
		return true
	})
	return out
}

func isStatusWord(expr ast.Expr) bool {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false
	}
	word, err := strconv.Unquote(lit.Value)
	return err == nil && leadMeetingStatusWords[word]
}

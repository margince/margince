// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/credentialmask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	typedSubject = "Zugang Passwort: Hunter2-Example!"
	typedBody    = "Hallo Pat,\nPasswort: Hunter2-Example!\nBis bald"
	typedSecret  = "Hunter2-Example!"
)

func TestAMailSendStoresMaskedTextAndDeliversWhatWasTyped(t *testing.T) {
	m := outboundMessage{
		in:           SendEmailInput{Subject: typedSubject, Body: typedBody},
		messageID:    "outbound-1@margince.test",
		body:         typedBody,
		recordedBody: typedBody,
		to:           []string{"pat@counterparty.test"},
	}
	row := m.activity(threading{})
	assertMaskedText(t, "timeline subject", row.Subject, typedSubject)
	assertMaskedText(t, "timeline body", row.Body, typedBody)

	delivery := m.delivery(ids.NewV7(), threading{}, SendOrigin{})
	for field, got := range map[string]string{
		"subject": delivery.Subject, "body": delivery.Body,
		"authored subject": delivery.AuthoredSubject, "authored body": delivery.AuthoredBody,
		"judged body": delivery.Authorization.Body,
	} {
		if !strings.Contains(got, typedSecret) {
			t.Errorf("delivery %s = %q, want the text as typed", field, got)
		}
	}
}

func TestAChannelReplyStoresMaskedTextAndDeliversWhatWasTyped(t *testing.T) {
	m := outboundChannelMessage{in: SendMessageInput{Body: typedBody}, provider: "telegram"}
	assertMaskedText(t, "timeline body", m.activity().Body, typedBody)
	if got := m.delivery(ids.From[ids.ActivityKind](ids.NewV7())).Body; got != typedBody {
		t.Errorf("delivery body = %q, want the text as typed", got)
	}
}

// assertMaskedText holds a stored field to the text capture would store for
// the same typed text, so a sent row and its synced copy read alike.
func assertMaskedText(t *testing.T, field string, stored *string, typed string) {
	t.Helper()
	if stored == nil {
		t.Fatalf("%s is unset", field)
	}
	if want := credentialmask.MaskText(typed); *stored != want || !strings.Contains(*stored, credentialmask.Marker) {
		t.Errorf("%s = %q, want %q", field, *stored, want)
	}
}

// Every literal row whose direction is fixed to outbound is a sent row, and it
// is built inside SentRecord or the mask is skipped. The census is read from
// the backend's own code, so a new builder is in it the day it is written.
func TestEverySentRowIsBuiltThroughSentRecord(t *testing.T) {
	root := filepath.Join("..", "..")
	var builders, unmasked []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
			return err
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			for _, row := range outboundRowLiterals(fn) {
				where := path + ": " + fn.Name.Name
				builders = append(builders, where)
				if !row.masked {
					unmasked = append(unmasked, where)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reading the backend: %v", err)
	}
	// The send, the channel reply and controller mail: fewer means the scan
	// stopped seeing builders, not that they went away.
	if len(builders) < 3 {
		t.Fatalf("found %d sent-row builders, want at least 3: %v", len(builders), builders)
	}
	for _, where := range unmasked {
		t.Errorf("%s builds an outbound activity row outside SentRecord, so its text is stored unmasked", where)
	}
}

type outboundRow struct{ masked bool }

// outboundRowLiterals lists the LogActivityInput literals in fn whose Direction
// is outbound, and whether each is the argument of a SentRecord call.
func outboundRowLiterals(fn *ast.FuncDecl) []outboundRow {
	assigned := assignedExprs(fn)
	var rows []outboundRow
	var stack []ast.Node
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if node == nil {
			stack = stack[:len(stack)-1]
			return true
		}
		if literal, ok := node.(*ast.CompositeLit); ok && isLogActivityInput(literal.Type) && hasOutboundDirection(literal, assigned) {
			rows = append(rows, outboundRow{masked: len(stack) > 0 && isSentRecordCall(stack[len(stack)-1])})
		}
		stack = append(stack, node)
		return true
	})
	return rows
}

func isLogActivityInput(expr ast.Expr) bool {
	switch typ := expr.(type) {
	case *ast.Ident:
		return typ.Name == "LogActivityInput"
	case *ast.SelectorExpr:
		return typ.Sel.Name == "LogActivityInput"
	}
	return false
}

func isSentRecordCall(node ast.Node) bool {
	call, ok := node.(*ast.CallExpr)
	return ok && strings.HasSuffix(types.ExprString(call.Fun), "SentRecord")
}

// hasOutboundDirection reads the Direction field, and the value its variable
// was assigned in the same function.
func hasOutboundDirection(literal *ast.CompositeLit, assigned map[string]ast.Expr) bool {
	for _, element := range literal.Elts {
		field, ok := element.(*ast.KeyValueExpr)
		if !ok || types.ExprString(field.Key) != "Direction" {
			continue
		}
		text := types.ExprString(field.Value)
		if unary, ok := field.Value.(*ast.UnaryExpr); ok {
			if name, ok := unary.X.(*ast.Ident); ok && assigned[name.Name] != nil {
				text += " " + types.ExprString(assigned[name.Name])
			}
		}
		return strings.Contains(text, `"outbound"`) || strings.Contains(text, "ActivityDirectionOutbound")
	}
	return false
}

func assignedExprs(fn *ast.FuncDecl) map[string]ast.Expr {
	assigned := map[string]ast.Expr{}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if assign, ok := node.(*ast.AssignStmt); ok && len(assign.Lhs) == len(assign.Rhs) {
			for i, lhs := range assign.Lhs {
				if name, ok := lhs.(*ast.Ident); ok {
					assigned[name.Name] = assign.Rhs[i]
				}
			}
		}
		return true
	})
	return assigned
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind reachability H2

package gates

// Every option that wires the mail relay is wired by a role binary.
//
// A compose option nobody calls is dead code, and most kinds announce
// themselves: the feature is simply absent and somebody notices. A MAILER
// option does not. The product still answers, still records the participant,
// still mints the credential — it just never sends, and the surface reports
// that as an ordinary "no relay configured", which is exactly what an
// installation without SMTP looks like.
//
// That shipped. WithDealRoomInviteMail had no caller in cmd/api, which wired
// only WithOperatorMail, so every deal room invitation in every real
// installation came back unsent while the startup banner said "password reset,
// invites". The seller was told to pass the link on by hand and had no way to
// know the product had not tried.
//
// Derived from the SIGNATURE, not a list: an option taking a mailer.Mailer is
// one of these, whatever it is named, so a third joins this gate the day it is
// written. What the gate does NOT check is that the relay reaches the handler
// the option claims — a fitness function can see that main calls the option,
// never that the option wires the right handler set. It makes an unwired one
// visible, which is the half that was silent.

import (
	"go/ast"
	"os"
	"path/filepath"
	"testing"
)

func TestEveryMailRelayOptionIsWiredByARoleBinary(t *testing.T) {
	t.Parallel()

	options := mailerOptionsIn(t, "internal/compose")
	if len(options) == 0 {
		t.Fatal("no compose option takes a mailer.Mailer, so this gate has no subject and would " +
			"pass however many relays went unwired")
	}

	roles, err := os.ReadDir("cmd")
	if err != nil {
		t.Fatalf("reading the process roles under cmd/: %v", err)
	}
	// composeCallsIn is bootcomposition_test's, selector-matched on the compose
	// package: one reader of "which options does a role call", asked here of a
	// different set of options.
	wired := map[string]bool{}
	for _, role := range roles {
		if !role.IsDir() {
			continue
		}
		for name := range composeCallsIn(t, filepath.Join("cmd", role.Name())) {
			wired[name] = true
		}
	}

	for _, option := range options {
		if !wired[option] {
			t.Errorf("compose.%s wires the mail relay but no role binary under cmd/ calls it. "+
				"An unwired mailer does not fail — the product records everything and silently "+
				"sends nothing, which reads to an operator exactly like an installation with no "+
				"SMTP configured. Call it where the relay is built, beside the other mail options.",
				option)
		}
	}
}

// mailerOptionsIn names every exported function in the package that takes a
// mailer.Mailer and returns an Option.
func mailerOptionsIn(t *testing.T, dir string) []string {
	t.Helper()
	// parseGoFilesUnder is jobrole_test's, which every gate here reaches for:
	// one reader of "the Go files under this directory", rather than a second
	// walk that would drift from it over build tags.
	_, files := parseGoFilesUnder(t, dir)

	var options []string
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !fn.Name.IsExported() {
				continue
			}
			if takesAMailer(fn) && returnsAnOption(fn) {
				options = append(options, fn.Name.Name)
			}
		}
	}
	return options
}

func takesAMailer(fn *ast.FuncDecl) bool {
	for _, param := range fn.Type.Params.List {
		selector, ok := param.Type.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		pkg, ok := selector.X.(*ast.Ident)
		if ok && pkg.Name == "mailer" && selector.Sel.Name == "Mailer" {
			return true
		}
	}
	return false
}

func returnsAnOption(fn *ast.FuncDecl) bool {
	if fn.Type.Results == nil {
		return false
	}
	for _, result := range fn.Type.Results.List {
		if name, ok := result.Type.(*ast.Ident); ok && name.Name == "Option" {
			return true
		}
	}
	return false
}

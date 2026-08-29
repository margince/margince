// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

//go:build !integration

package gates

// A request path reaches a log line through capabilitypath.Redact, never raw.
//
// Some public routes carry a bearer credential in a path segment, because they
// are reached with no login: the recipient of an email follows a link, and the
// token in it IS the authorization. The preference centre's token reads and
// changes a person's consent state; the confirm-details token opens the
// subject's own record. A log line holding one is a working credential sitting
// wherever logs go — an ops dashboard, a shipped aggregator, a third-party log
// service, a support engineer's paste.
//
// The access log knew this and redacted. Six other sites did not, and they
// wrote the same paths: an unhandled error, an infrastructure fault, a withheld
// error detail, a body that could not be read, and two field-level decode
// refusals. Any 500 on one of those routes wrote the credential out. The holder
// of such a URL could trigger it, and so could anyone who could provoke a fault
// on that path.
//
// A PROHIBITION rather than a census, because the failure is a leak by default:
// `"path", r.URL.Path` is what an author reaches for, it is correct-looking at
// every one of the seven sites, and the next log line inherits it. The rule is
// that a path becomes a log value only through the redactor.
//
// What this cannot see, stated rather than implied. The walk is syntactic, so
// it catches the shape actually written here and the shape the next author
// would reach for: the path read inside the argument list of a log call. It
// does NOT follow a path assigned to a variable first, formatted into a message
// with fmt.Sprintf, or carried into a helper this walk does not enter. Those
// launder the value out of the gate's sight, and only a reviewer catches them.

import (
	"go/ast"
	"os"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// The log doors a path must not reach raw. slog's package-level functions and
// its *Logger methods share these names, so matching the selector name alone
// covers both — and a name like `Warn` on some unrelated receiver would only
// ever produce a finding if it were ALSO handed r.URL.Path, which is the
// defect regardless of who logs it.
var logDoors = map[string]bool{
	"Debug": true, "Info": true, "Warn": true, "Error": true,
	"DebugContext": true, "InfoContext": true, "WarnContext": true, "ErrorContext": true,
	"Log": true, "LogAttrs": true,
}

// rawPathRead reports an expression that reads .Path off a *http.Request.
//
// Matched on the selector chain rather than on a resolved type: the gates
// package parses without type information, and `x.URL.Path` is the shape in
// every one of these call sites. A request named something other than `r` is
// still caught, because the chain and not the identifier is what is matched.
func rawPathRead(expr ast.Expr) bool {
	path, ok := expr.(*ast.SelectorExpr)
	if !ok || path.Sel.Name != "Path" {
		return false
	}
	url, ok := path.X.(*ast.SelectorExpr)
	return ok && url.Sel.Name == "URL"
}

// logCallLeakingAPath reports the log doors in this file that are handed a raw
// request path as an argument.
func logCallLeakingAPath(file *ast.File) []string {
	var leaks []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		door, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !logDoors[door.Sel.Name] {
			return true
		}
		for _, arg := range call.Args {
			if rawPathRead(arg) {
				leaks = append(leaks, door.Sel.Name)
			}
		}
		return true
	})
	return leaks
}

func TestNoRequestPathReachesALogLineUnredacted(t *testing.T) {
	t.Parallel()

	files := gatekit.Scope{
		Roots: []string{"internal"},
		// Not extensions/: no unit logs a request path today, and a root that
		// finds no subject fails this scope rather than passing quietly. An
		// extension that starts logging one is served by the same rule and
		// belongs in this list on the day it does.
		Subject: fileLogsARequestPath,
	}.Files(t)

	judged := 0
	for _, parsed := range files {
		// capabilitypath owns the redaction, and its own test states the paths
		// it is redacting. Excluded by package rather than by file so a second
		// file in it is covered without an edit here.
		if strings.Contains(parsed.Path, "/shared/kernel/capabilitypath/") {
			continue
		}
		judged++
		for _, door := range logCallLeakingAPath(parsed.File) {
			t.Errorf("%s: slog %s is handed r.URL.Path directly\n"+
				"\tA public route can carry a bearer credential in a path segment, so a raw "+
				"path in a log line can be a working credential wherever logs are read.\n"+
				"\tPass it through shared/kernel/capabilitypath.Redact, which knows which "+
				"prefixes carry one.", parsed.Path, door)
		}
	}
	// The count is taken AFTER the exclusion. A walk that found only
	// capabilitypath itself — or one whose subject test broke and matched
	// nothing — would otherwise report a swept, successful, empty run, which is
	// the one way this gate must not fail. Something in this tree logs a
	// request path: the access log does it on every request.
	if judged == 0 {
		t.Fatal("no file outside capabilitypath logs a request path, so this prohibition " +
			"judged nothing — the walk is broken, or the log doors were renamed and this " +
			"rule now binds calls nobody makes")
	}
}

// Accepting httperr's loggedPath(r) as already-redacted is only sound while
// loggedPath actually redacts. Nothing else in this walk can see inside it, so
// the one wrapper this gate trusts is pinned here by name: strip the redaction
// out of it and every httperr log line leaks with the gate still green.
func TestTheOneTrustedPathWrapperStillRedacts(t *testing.T) {
	t.Parallel()

	const wrapper = "internal/platform/httperr/httperr.go"
	body, err := os.ReadFile(wrapper)
	if err != nil {
		t.Fatalf("reading the trusted path wrapper: %v", err)
	}
	src := string(body)
	if !strings.Contains(src, "func loggedPath(r *http.Request) string {") {
		t.Fatalf("%s no longer declares loggedPath; this gate trusts that name as "+
			"already-redacted, so either restore it or drop it from redactedPathRead", wrapper)
	}
	declaration, _, _ := strings.Cut(src[strings.Index(src, "func loggedPath"):], "\n}")
	if !strings.Contains(declaration, "capabilitypath.Redact(r.URL.Path)") {
		t.Errorf("%s: loggedPath no longer passes the path through capabilitypath.Redact, "+
			"so every httperr log line that calls it writes the raw path while this gate "+
			"reads them as fixed:\n%s", wrapper, declaration)
	}
}

// fileLogsARequestPath selects the files worth judging: those that both log
// and read a request path. Selecting on the log door alone would drag in every
// file that logs anything; selecting on the path alone would drag in every
// router.
func fileLogsARequestPath(_ string, file *ast.File) bool {
	logs, readsPath := false, false
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if door, ok := call.Fun.(*ast.SelectorExpr); ok && logDoors[door.Sel.Name] {
			logs = true
		}
		for _, arg := range call.Args {
			if rawPathRead(arg) || redactedPathRead(arg) {
				readsPath = true
			}
		}
		return true
	})
	return logs && readsPath
}

// redactedPathRead reports a path that already goes through the redactor, so a
// file that has been fixed STAYS in the gate's corpus. Without this, fixing the
// last leaking file would shrink the corpus to nothing and the empty-sweep
// guard would fire on a clean tree.
//
// Two shapes count. `capabilitypath.Redact(r.URL.Path)` at the call is the
// direct one. A one-argument wrapper taking the request — httperr's
// loggedPath(r) — is the other: httperr logs six paths and spelling the
// redaction out six times invites the seventh to forget, but the wrapper hides
// the .URL.Path read from this walk.
func redactedPathRead(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	if fn, ok := call.Fun.(*ast.SelectorExpr); ok && fn.Sel.Name == "Redact" {
		for _, arg := range call.Args {
			if rawPathRead(arg) {
				return true
			}
		}
	}
	fn, ok := call.Fun.(*ast.Ident)
	return ok && fn.Name == "loggedPath"
}

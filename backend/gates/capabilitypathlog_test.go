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
// Two things are checked, because redacting at every site proves nothing if
// the list of routes needing redaction is short. The prohibition holds the
// sites; TestEveryPublicRoutePrefixIsJudgedForCredentials holds the list,
// reading the public route declarations and requiring each to be redacted or
// to carry a written reason it is not.
//
// What this cannot see, stated rather than implied. The walk is syntactic. It
// catches a raw path in a log call's arguments, and a helper that returns one
// (the shape that would otherwise hide a read from the argument walk). It does
// NOT follow a path assigned to a variable first, concatenated, or formatted
// through fmt.Sprintf. Those launder the value out of the gate's sight, and
// only a reviewer catches them.

import (
	"go/ast"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/capabilitypath"
)

const capabilityPathPkg = "github.com/margince/margince/backend/internal/shared/kernel/capabilitypath"

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
func rawPathRead(node ast.Node) bool {
	path, ok := node.(*ast.SelectorExpr)
	if !ok || path.Sel.Name != "Path" {
		return false
	}
	url, ok := path.X.(*ast.SelectorExpr)
	return ok && url.Sel.Name == "URL"
}

// logCallLeakingAPath reports the log doors in this file that are handed a raw
// request path as an argument.
//
// A function in this file that RETURNS a raw path counts as the same leak, and
// is reported at the function rather than at the call. That is the shape the
// argument walk cannot see: the whole reason one wrapper is trusted is that it
// hides the read, so a second wrapper hides one just as well.
func logCallLeakingAPath(file *ast.File, path string) []string {
	var leaks []string
	ast.Inspect(file, func(n ast.Node) bool {
		if fn, ok := n.(*ast.FuncDecl); ok && returnsARawPath(fn) &&
			!(fn.Name.Name == trustedPathWrapper && path == trustedPathWrapperFile) {
			leaks = append(leaks, "the helper "+fn.Name.Name+" returns r.URL.Path unredacted")
			return true
		}
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
				leaks = append(leaks, "slog "+door.Sel.Name+" is handed r.URL.Path directly")
			}
		}
		return true
	})
	return leaks
}

// returnsARawPath reports a function whose return statement hands back
// r.URL.Path with nothing done to it.
func returnsARawPath(fn *ast.FuncDecl) bool {
	found := false
	ast.Inspect(fn, func(n ast.Node) bool {
		ret, ok := n.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		for _, result := range ret.Results {
			if rawPathRead(result) {
				found = true
			}
		}
		return true
	})
	return found
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
		for _, leak := range logCallLeakingAPath(parsed.File, parsed.Path) {
			t.Errorf("%s: %s\n"+
				"\tA public route can carry a bearer credential in a path segment, so a raw "+
				"path in a log line can be a working credential wherever logs are read.\n"+
				"\tPass it through shared/kernel/capabilitypath.Redact, which knows which "+
				"prefixes carry one.", parsed.Path, leak)
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

	const wrapper = trustedPathWrapperFile
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

// Redacting correctly at every site proves nothing if the list of routes that
// need it is short: every call still runs, and Redact hands the credential
// straight back for a route it does not know. So the public prefixes are read
// off the route declarations themselves — `const public<Name>Prefix = "…"` in
// compose — and each is accounted for either as redacted or as a deliberate
// exclusion carrying its reason.
//
// This is the half a syntactic walk cannot reach on its own, and the half that
// fails silently: a new token-in-path route added tomorrow leaks with the
// prohibition above still green.
func TestEveryPublicRoutePrefixIsJudgedForCredentials(t *testing.T) {
	t.Parallel()

	// The public prefixes whose segment is NOT a credential, each with why.
	// A prefix leaves this map only by joining capabilitypath's list.
	notCredentials := map[string]string{
		"/v1/public/rooms/": "the segment is an operation name (peek, exchange, link-request); a deal room presents a Bearer, never a path token",
	}

	redacted := make(map[string]bool)
	for _, prefix := range capabilitypath.CredentialPrefixes() {
		redacted[prefix] = true
	}

	declared := publicRoutePrefixes(t)
	if len(declared) == 0 {
		t.Fatal("no public route prefix was found in compose, so this parity check judged " +
			"nothing — the declaration shape changed and this gate now certifies an empty set")
	}
	for prefix, where := range declared {
		reason, excluded := notCredentials[prefix]
		switch {
		case redacted[prefix] && excluded:
			t.Errorf("%s: %q is both redacted and listed as not-a-credential (%q) — one of the two is wrong",
				where, prefix, reason)
		case !redacted[prefix] && !excluded:
			t.Errorf("%s: the public route %q is neither redacted nor accounted for.\n"+
				"\tIf its next path segment is a bearer credential, add it to credentialPrefixes in "+
				"shared/kernel/capabilitypath — every log site already calls Redact, and Redact returns "+
				"an unknown route's credential unchanged.\n"+
				"\tIf it is a public identifier, say so in notCredentials here with the reason.",
				where, prefix)
		}
	}
	for prefix := range notCredentials {
		if _, ok := declared[prefix]; !ok {
			t.Errorf("notCredentials names %q, which no compose route declares any more — "+
				"drop the entry rather than leaving a stale exclusion standing", prefix)
		}
	}
}

// publicRoutePrefixes reads the `const public<Name>Prefix = "/v1/public/…"`
// declarations out of compose, keyed by prefix, valued by where it was found.
// Derived from the declarations rather than listed here, so a new public route
// enters this check the moment it exists.
func publicRoutePrefixes(t *testing.T) map[string]string {
	t.Helper()

	found := map[string]string{}
	for _, parsed := range (gatekit.Scope{
		Roots:   []string{"internal"},
		Subject: declaresAPublicRoutePrefix,
	}).Files(t) {
		for name, value := range publicPrefixConstants(parsed.File) {
			found[value] = parsed.Path + ": " + name
		}
	}
	return found
}

// publicPrefixConstants reads this file's `public<Name>Prefix = "/v1/public/…"`
// declarations, by constant name.
func publicPrefixConstants(file *ast.File) map[string]string {
	out := map[string]string{}
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok || len(spec.Names) != 1 || len(spec.Values) != 1 {
			return true
		}
		name := spec.Names[0].Name
		if !strings.HasPrefix(name, "public") || !strings.HasSuffix(name, "Prefix") {
			return true
		}
		lit, ok := spec.Values[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(lit.Value)
		if err != nil || !strings.HasPrefix(value, "/v1/public/") {
			return true
		}
		out[name] = value
		return true
	})
	return out
}

func declaresAPublicRoutePrefix(_ string, file *ast.File) bool {
	return len(publicPrefixConstants(file)) > 0
}

// fileLogsARequestPath selects the files worth judging: those that both log
// and read a request path. Selecting on the log door alone would drag in every
// file that logs anything; selecting on the path alone would drag in every
// router.
// A file qualifies on reading a request path ANYWHERE, not only inside a log
// call's arguments. Requiring the read to sit in the call would let a file drop
// out of the corpus by moving the read into a helper — which is the shape that
// hides a leak, so it must select the file IN rather than out.
func fileLogsARequestPath(_ string, file *ast.File) bool {
	logs, readsPath := false, false
	ast.Inspect(file, func(n ast.Node) bool {
		if rawPathRead(n) {
			readsPath = true
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if door, ok := call.Fun.(*ast.SelectorExpr); ok && logDoors[door.Sel.Name] {
			logs = true
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
// Two shapes count, and neither is matched by bare name. `Redact` and
// `loggedPath` are ordinary identifiers, so an unrelated package defining
// either — a `loggedPath` that returns the path raw is the obvious one — would
// be read as already-fixed and leak with this gate green.
//
// `capabilitypath.Redact(r.URL.Path)` at the call is the direct shape,
// resolved through the file's own import of the redactor.
//
// The wrapper shape is the other: httperr logs six paths, and spelling the
// redaction out six times invites the seventh to forget, but a wrapper hides
// the .URL.Path read from this walk. Exactly ONE wrapper is trusted, and only
// inside the single file that declares it — trusting the name package-wide, or
// wherever the redactor happens to be imported, is what lets a second
// same-named helper borrow the trust. TestTheOneTrustedPathWrapperStillRedacts
// pins what that one wrapper does.
const (
	trustedPathWrapper     = "loggedPath"
	trustedPathWrapperFile = "internal/platform/httperr/httperr.go"
)

func redactedPathRead(file *ast.File, path string, expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	if fn, ok := call.Fun.(*ast.SelectorExpr); ok && fn.Sel.Name == "Redact" {
		qualifier, _ := gatekit.ImportedAs(file, capabilityPathPkg)
		pkg, isIdent := fn.X.(*ast.Ident)
		if isIdent && qualifier != "" && pkg.Name == qualifier {
			for _, arg := range call.Args {
				if rawPathRead(arg) {
					return true
				}
			}
		}
	}
	fn, ok := call.Fun.(*ast.Ident)
	return ok && fn.Name == trustedPathWrapper && path == trustedPathWrapperFile
}

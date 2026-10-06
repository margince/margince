// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

//go:build !integration

package gates

// A 5xx the api sends carries a problem body, or it says which surface it is on.
//
// The web client reads a bare 502, 503 or 504 from /v1 — one with no
// `application/problem+json` body — as the PATH to the api having failed rather
// than the api answering: a proxy in front of a process that is down. It shows
// the connectivity banner and pauses reads until /healthz answers
// (frontend/src/api/client.ts, docs/explanation/pwa.md). A 5xx that carries a
// problem body counts as the api answering, which is what keeps an ordinary
// server error from looking like an outage.
//
// That rests on every /v1 answer going through httperr, and nothing said so. A
// handler writing a plain http.Error or a bare WriteHeader(500) would read to
// every client as the api being unreachable.
//
// WHY THE WHOLE TREE AND NOT THE /v1 PACKAGES. The contract surface is ONE
// generated router over compose.Server, which delegates into a dozen module
// packages; a list of "the packages under /v1" would be a second copy of that
// call graph, and a stale one the day a handler moves. Scanning every
// hand-written writer and making each bare 5xx name the non-/v1 surface it
// serves asks the same question from the other end, and cannot fail short: a
// /v1 handler has no true answer to give.
//
// http.Error is forbidden outright rather than only for 5xx. It writes
// text/plain whatever the status, so it is off-contract everywhere on this
// surface, and there is no hand-written call left to keep.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// bareWriter is a file:line, which is what a reader needs to go and look.
type bareWriter string

// bareFiveHundreds are the answers written without a problem body, each naming
// the surface it serves. A /v1 route cannot appear here truthfully: the client
// classifies exactly that prefix, so a bare 5xx under it is the outage signal.
var bareFiveHundreds = gatekit.Waive(map[bareWriter]string{
	"internal/platform/httpserver/observe.go": "the readiness probe on the OBSERVE listener, which is not the api port at " +
		"all. Its reader is an orchestrator that wants a status line and a dependency name, and it is served as plain text " +
		"on purpose — a problem document would be a body no probe parses",
	"internal/compose/webhook.go": "the provider webhook surface at /webhooks/, whose reader is the sending provider and " +
		"whose 500 is a RETRY instruction rather than a message to a human. Providers redeliver on 5xx and parse no body",
	"internal/compose/extinbound.go": "the extension inbound surface at /webhooks/ext/, the same contract as the webhook " +
		"door beside it: the caller is a provider that redelivers on 5xx, and the unit behind the path is not part of the " +
		"generated /v1 router",
})

var (
	// bareFiveHundred is a WriteHeader whose status is a 5xx, by constant or by
	// number. Both spellings, because a gate that read only the constants would
	// pass the first literal somebody writes.
	bareFiveHundred = regexp.MustCompile(
		`WriteHeader\(\s*(?:http\.Status(?:InternalServerError|NotImplemented|BadGateway|ServiceUnavailable|GatewayTimeout|HTTPVersionNotSupported|VariantAlsoNegotiates|InsufficientStorage|LoopDetected|NotExtended|NetworkAuthenticationRequired)|5\d\d)\s*\)`)
	// plainHTTPError is net/http's text/plain writer at any status.
	plainHTTPError = regexp.MustCompile(`\bhttp\.Error\(`)
)

// bareWriterFloor is the smallest number of Go files this gate may read and
// still be believed. Its job is to catch a walk that stopped walking, which
// would report a clean tree from nothing.
const bareWriterFloor = 1500

func TestEveryBare5xxNamesASurfaceThatIsNotTheContract(t *testing.T) {
	t.Parallel()

	read := 0
	for _, root := range []string{"internal", "cmd", "pkg"} {
		walkGoFiles(t, root, func(rel string, body []byte) {
			read++
			text := string(body)
			if plainHTTPError.MatchString(text) {
				t.Errorf("%s calls http.Error, which writes text/plain at any status. On /v1 that is an "+
					"answer no client can read as the api answering; write it through httperr", rel)
			}
			if !bareFiveHundred.MatchString(text) {
				return
			}
			if !bareFiveHundreds.Waived(t, bareWriter(rel)) {
				t.Errorf("%s writes a 5xx header with no problem body. Under /v1 the web client reads that "+
					"as the api being unreachable and pauses every read; write it through httperr, or declare "+
					"in bareFiveHundreds which non-contract surface this one serves", rel)
			}
		})
	}
	if read < bareWriterFloor {
		t.Fatalf("read only %d Go file(s) and expects at least %d — the walk is broken, not the tree", read, bareWriterFloor)
	}
	bareFiveHundreds.AssertAllMatched(t)
}

// walkGoFiles hands each hand-written Go file under root to fn, by its path
// relative to the backend module. Generated files and tests are left out: the
// generated router is not somebody's choice to change, and a test's own stub
// server answers nobody's client.
func walkGoFiles(t *testing.T, root string, fn func(rel string, body []byte)) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		if strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
			return nil
		}
		body, err := os.ReadFile(path) // #nosec G304 -- a *.go file from walking the trusted backend tree
		if err != nil {
			return err
		}
		fn(path, body)
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}
}

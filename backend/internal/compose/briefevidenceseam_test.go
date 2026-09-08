// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// emailReaderNotNeeded ratifies a construction that reaches no code path which
// would ask the reader anything. Each is a sentence somebody wrote, and
// AssertAllMatched fails a waiver whose construction has since gone — so a
// ratified worker that later grows a reader-side call is not silently exempt.
var emailReaderNotNeeded = gatekit.Waive(map[string]string{
	"jobs_accountscan.go:companyscan:newAccountScanWorker": "the worker role only ever calls Run, which WRITES findings; the summaries are attached by wire on the reader's side, out of the reader's own grants, so a reader here would be wired to a path that never asks it anything",
})

// A service that grounds prose in records is constructed in two places — the
// default assembly, and the option that rebinds it once a model lane is wired —
// and BOTH must be handed the reader that opens a cited message.
//
// The omission is silent, which is why this exists. A service built without the
// reader draws exactly what it drew before: the citations are there, the prose
// is there, and the chips do nothing. No error, no empty section, no failing
// unit test — the enrichment is a no-op by design so that a service under unit
// test needs no database. So the one thing that can catch a forgotten
// rebinding is a census over the constructions themselves.
//
// It derives its corpus from the services that DECLARE WithEmailSummaries
// rather than from a list written here: a tenth producer added next year is
// judged the moment it declares the builder, and a producer that loses the
// builder stops being judged for the right reason.
func TestEveryGroundedServiceConstructionTakesTheEmailReader(t *testing.T) {
	t.Parallel()
	defer emailReaderNotNeeded.AssertAllMatched(t)
	declaring := servicesDeclaringEmailSummaries(t)
	// A census that can fail short has already failed: with an empty corpus
	// this walks nothing and reports PASS over every construction at once.
	if len(declaring) == 0 {
		t.Fatal("no compose service declares WithEmailSummaries — either the builder was " +
			"renamed or the enrichment was removed, and this census is now judging nothing")
	}
	for _, file := range composeSources(t) {
		source := readSource(t, file)
		for _, pkg := range declaring {
			for _, construction := range constructionsOf(source, pkg) {
				if strings.Contains(construction, "WithEmailSummaries") {
					continue
				}
				subject := fmt.Sprintf("%s:%s:%s", filepath.Base(file), pkg, enclosingFunc(source, construction))
				if emailReaderNotNeeded.Waived(t, subject) {
					continue
				}
				t.Errorf("%s constructs %s without WithEmailSummaries — the citations it "+
					"writes will render as chips that open nothing, and no other test fails",
					filepath.Base(file), pkg)
			}
		}
	}
}

// servicesDeclaringEmailSummaries reads which compose subpackages own a
// WithEmailSummaries builder, from the subpackages themselves.
func servicesDeclaringEmailSummaries(t *testing.T) []string {
	t.Helper()
	var found []string
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the compose directory: %v", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		matches, err := filepath.Glob(filepath.Join(entry.Name(), "*.go"))
		if err != nil {
			t.Fatalf("listing %s: %v", entry.Name(), err)
		}
		for _, file := range matches {
			if strings.Contains(readSource(t, file), ") WithEmailSummaries(") {
				found = append(found, entry.Name())
				break
			}
		}
	}
	return found
}

// composeSources are compose's own non-test files, which is where every
// production construction lives.
func composeSources(t *testing.T) []string {
	t.Helper()
	matches, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("listing compose: %v", err)
	}
	var out []string
	for _, file := range matches {
		if !strings.HasSuffix(file, "_test.go") {
			out = append(out, file)
		}
	}
	return out
}

// constructionsOf cuts out each construction of a SERVICE in this package,
// together with the builder chain that follows it.
//
// `NewHandlers` is excluded deliberately: a handler set takes a service that is
// already built, so demanding the builder there would fail every correct
// wiring. What this looks for is the constructor that produces the thing the
// builder hangs off.
//
// The chain runs until the constructor's own brackets are balanced AND the last
// line no longer ends in a dot — which is how `NewService(\n ...args...\n).\n
// WithEmailSummaries(...)` is read as one construction rather than as a bare
// constructor followed by an unrelated line. Reading too FAR is the dangerous
// direction, because a neighbour's builder would then vouch for this one, so
// the walk also stops at the first line that starts a new statement.

func constructionsOf(source, pkg string) []string {
	opener := regexp.MustCompile(`\b` + regexp.QuoteMeta(pkg) + `\.New(?:Handlers\b)?([A-Za-z]*)\(`)
	var out []string
	lines := strings.Split(source, "\n")
	for i, line := range lines {
		match := opener.FindStringSubmatch(line)
		if match == nil || strings.Contains(line, pkg+".NewHandlers(") {
			continue
		}
		chain := []string{line}
		depth := strings.Count(line, "(") - strings.Count(line, ")")
		for j := i + 1; j < len(lines) && (depth > 0 || endsChain(lines[j-1])); j++ {
			chain = append(chain, lines[j])
			depth += strings.Count(lines[j], "(") - strings.Count(lines[j], ")")
		}
		out = append(out, strings.Join(chain, "\n"))
	}
	return out
}

// endsChain says a line hands on to a builder on the next one.
func endsChain(line string) bool {
	return strings.HasSuffix(strings.TrimSpace(line), ".")
}

// enclosingFunc names the function a construction sits in, so a waiver ratifies
// ONE construction rather than every construction of that package in the file.
func enclosingFunc(source, construction string) string {
	at := strings.Index(source, construction)
	if at < 0 {
		return "?"
	}
	declaration := regexp.MustCompile(`(?m)^func (?:\([^)]*\) )?([A-Za-z0-9_]+)\(`)
	name := "?"
	for _, match := range declaration.FindAllStringSubmatchIndex(source[:at], -1) {
		name = source[match[2]:match[3]]
	}
	return name
}

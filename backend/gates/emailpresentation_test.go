// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A retained email reads the same everywhere, or it does not read the same
// anywhere.
//
// The tree once held five independent renderings of one message — the
// timeline's, the account page's recent list, the person memory's fold, the
// relationship spine's and search's — each deciding for itself which parts of
// an email to show and whether it could be opened. They drifted, because
// nothing failed when they did: a surface that grew a sixth reading looked
// exactly like one reusing the canonical row.
//
// So the contract names which of its schemas ARE an email, in the schema
// itself:
//
//	x-email-presentation: entry    — this is a message, and it is rendered by
//	                                 the canonical components
//	x-email-presentation: exempt   — with x-email-presentation-reason stating
//	                                 why this one is not
//
// and this census holds the two halves against each other. It reads the
// annotation from `api/crm.yaml` and the consumers from `frontend/src`, and
// fails in BOTH directions: an `entry` schema reaching a file that is not a
// canonical renderer, and a canonical renderer that has stopped consuming any
// `entry` schema at all.
//
// The second direction is the one worth having. A gate that only checked
// "nothing else renders an email" would report PASS over a tree where the
// canonical row had been deleted and every surface had gone back to its own
// spelling — the exact state this arc was built to end, reported as clean.
//
// A file satisfies this gate by ONE of three means:
//
//   - it is a canonical renderer, named in emailRenderers below;
//   - it merely PASSES the type through to one — a host that declares a prop
//     and hands it on renders nothing itself, and is admitted by name;
//   - it is a test, a story or the generated schema, which describe the
//     components rather than adding a rendering.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	emailContractPath   = "api/crm.yaml"
	emailFrontendSource = "../frontend/src"
)

// emailRenderers are the components allowed to draw a message FROM ITS
// CONTRACT TYPE: EmailEntry shows a message as a row, EmailDetail is the drawer
// the row opens into.
//
// EmailReference is deliberately NOT here. It cites a message rather than
// showing one, and it takes a subject and a date as plain scalars — no contract
// type at all, which is what stops a citation from ever growing a preview. A
// gate expecting it to consume an `entry` schema would be asserting the
// opposite of the design.
//
// Named rather than derived, because this is the decision the census exists to
// hold: a third renderer is exactly the thing that must not appear quietly, so
// adding one here is the deliberate act of widening the rule.
var emailRenderers = map[string]bool{
	"design-system/emailentry.tsx":  true,
	"design-system/emaildetail.tsx": true,
}

// emailPassthroughs hold an `entry` type without rendering it: they declare a
// prop and hand it to a canonical component. Each is a sentence somebody wrote
// rather than an omission nobody noticed.
var emailPassthroughs = gatekit.Waive(map[string]string{
	"design-system/composed.tsx": "declares TimelineEntry.emailSummary and passes it to EmailEntry; the timeline row chooses WHICH component draws a kind, and drawing none itself is what keeps that choice in one place",
})

// entrySchemas reads the schemas the contract marks as an email.
//
// It matches the ANNOTATION and walks back to the schema name, rather than
// matching a name pattern: a schema called EmailFoo that carries no annotation
// is not this gate's business, and one called Correspondence that carries
// `entry` is. Deriving the corpus from the marker is what stops the census
// answering about a set the contract never declared.
func entrySchemas(t *testing.T) []string {
	t.Helper()
	source, err := os.ReadFile(emailContractPath)
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	var names []string
	var current string
	schemaLine := regexp.MustCompile(`^    ([A-Za-z][A-Za-z0-9]*):\s*$`)
	for _, line := range strings.Split(string(source), "\n") {
		if match := schemaLine.FindStringSubmatch(line); match != nil {
			current = match[1]
			continue
		}
		if strings.TrimSpace(line) == "x-email-presentation: entry" && current != "" {
			names = append(names, current)
		}
	}
	return names
}

// consumersOf answers every frontend file naming this schema's generated type.
//
// The generated spelling is `components["schemas"]["Name"]`, which is how a
// TypeScript file reaches a contract type — there is no other way in, so a
// consumer this misses is a consumer that cannot exist.
func consumersOf(t *testing.T, schema string) []string {
	t.Helper()
	needle := fmt.Sprintf(`components["schemas"]["%s"]`, schema)
	var found []string
	err := filepath.WalkDir(emailFrontendSource, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".ts") && !strings.HasSuffix(path, ".tsx") {
			return nil
		}
		source, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(source), needle) {
			rel, relErr := filepath.Rel(emailFrontendSource, path)
			if relErr != nil {
				return relErr
			}
			found = append(found, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the frontend sources: %v", err)
	}
	return found
}

// describesRatherThanRenders is a file that talks ABOUT the components: a test,
// a story, or the generated schema itself. None of them is a second rendering.
func describesRatherThanRenders(path string) bool {
	return strings.HasPrefix(path, "api/") ||
		strings.Contains(path, ".test.") ||
		strings.Contains(path, ".stories.")
}

// An `entry` schema reaches the canonical components and nothing else.
func TestAnEmailIsDrawnOnlyByTheCanonicalComponents(t *testing.T) {
	t.Parallel()
	defer emailPassthroughs.AssertAllMatched(t)
	schemas := entrySchemas(t)
	// A census that can fail short has already failed: an annotation whose
	// spelling drifted would leave this walking an empty set and reporting
	// PASS over every surface at once.
	if len(schemas) == 0 {
		t.Fatal("no schema in the contract carries `x-email-presentation: entry` — " +
			"either the annotation was removed or its spelling drifted, and this census " +
			"is now judging nothing")
	}
	for _, schema := range schemas {
		for _, consumer := range consumersOf(t, schema) {
			if emailRenderers[consumer] || describesRatherThanRenders(consumer) {
				continue
			}
			if emailPassthroughs.Waived(t, consumer) {
				continue
			}
			t.Errorf("%s consumes %s and is not a canonical renderer — render EmailEntry "+
				"or EmailReference instead, or ratify the file in emailPassthroughs with "+
				"what it does with the type", consumer, schema)
		}
	}
}

// And the other direction: each canonical renderer still consumes one.
//
// Without this the census passes over a tree where EmailEntry has been gutted
// and every surface has gone back to its own rendering — nothing consumes an
// `entry` schema, so nothing is unauthorized, so the gate reports clean about
// the exact state it exists to prevent.
func TestEachCanonicalRendererStillDrawsAnEmail(t *testing.T) {
	t.Parallel()
	schemas := entrySchemas(t)
	drawing := map[string]bool{}
	for _, schema := range schemas {
		for _, consumer := range consumersOf(t, schema) {
			drawing[consumer] = true
		}
	}
	for renderer := range emailRenderers {
		if !drawing[renderer] {
			t.Errorf("%s is named as a canonical email renderer and consumes no schema marked "+
				"`x-email-presentation: entry` — it has stopped drawing a message, and every "+
				"surface that reused it has stopped too", renderer)
		}
	}
}

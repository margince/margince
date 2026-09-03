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
// A surface reaches an email by EITHER route, and the census must see both.
// Naming the generated type is one. Reading the property that carries it off a
// record already in hand — `row.email_summary.subject` on an Activity — is the
// other, and it names no type at all. An earlier version of this gate looked
// only for the type name and argued in a comment that there was no other way
// in. Three surfaces were drawing their own email rows while it reported PASS:
// the person page's memory card, the account page's recent list and the held
// threads table. Under-recognition is the one way a census must not break,
// because a smaller corpus fails silently and reads exactly like a clean tree.
//
// What this still cannot see, stated so the next reader does not assume
// otherwise: a surface that draws an email from the ACTIVITY's own scalars —
// `activity.subject`, `activity.body` — without ever touching the carrier
// field. It reads a message and names nothing an email, so no derivation
// reaches it. The account page's recent list and the held threads table were
// both in that shape. They are fixed, and what holds them is a rendering test
// each rather than this census; a fourth such surface would arrive unseen.
// Widening to activity scalars would put `.subject` — which notes, calls and
// tasks all carry — in front of a rule about email, and a census that flags
// every kind teaches a reader to waive it.
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

// emailPassthroughs hold an `entry` type without rendering it: they carry the
// message toward a canonical component without drawing any part of it. Each is
// a sentence somebody wrote rather than an omission nobody noticed.
//
// A file that MOUNTS a canonical component needs no entry here — it discharges
// the obligation by doing the thing, and is admitted by rendersCanonically.
// These are the ones that never mount anything: builders, mappers and the
// surfaces whose only act is to suppress their own title in favour of the row.
var emailPassthroughs = gatekit.Waive(map[string]string{
	"design-system/activitytimeline.tsx": "maps an Activity onto a TimelineEntry, email_summary included, for composed.tsx to draw; it builds the entry and renders nothing",
	"screens/openemail.ts":               "the drawer controller: it reads emailSummary only to decide an entry HAS a message to open, and holds no part of one",
	"screens/recordchronology.tsx":       "wires onOpenEmail onto the entries it hands to the timeline; the rendering is composed.tsx's",
	"screens/worklist.focus.tsx":         "suppresses its own title when the item carries a summary, leaving the row to WaitingEmailLine; the branch is the whole of its involvement",
	"screens/worklist.row.tsx":           "same branch as the focus card, for the list row",
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

// carrierFields answers the property names under which an `entry` schema hangs
// off another object — `email_summary` today, and whatever a later property is
// called, because the names are read from the contract rather than written here.
//
// This is the half a name-only census cannot see. A surface reaches an email
// through the record it is already holding — `row.email_summary.subject` off an
// Activity — and never writes the generated type at all, so it consumes a
// message while naming nothing this gate was looking for.
func carrierFields(t *testing.T, schemas []string) []string {
	t.Helper()
	source, err := os.ReadFile(emailContractPath)
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	marked := map[string]bool{}
	for _, schema := range schemas {
		marked[schema] = true
	}
	// A carrier is a property at the schema's OWN property depth holding a ref
	// to a marked schema, on a schema the contract says nothing about.
	//
	// Each half of that excludes a different thing. A response body's
	// `content:` is not a property, and admitting it would put `.content` — a
	// word half the tree writes — before the census as an email. A schema that
	// carries an annotation of its own is already judged: `EmailPresentation`
	// holding its `summary`, and the `exempt` EmailThreadPage holding its
	// `members`, are the drawer reading its own parts rather than a foreign
	// record carrying a message.
	schemaLine := regexp.MustCompile(`^    ([A-Za-z][A-Za-z0-9]*):\s*$`)
	propertyLine := regexp.MustCompile(`^        ([a-z][a-z0-9_]*):\s*$`)
	refLine := regexp.MustCompile(`#/components/schemas/([A-Za-z][A-Za-z0-9]*)`)
	seen := map[string]bool{}
	var fields []string
	var current string
	judged := false
	for _, line := range strings.Split(string(source), "\n") {
		if schemaLine.MatchString(line) {
			current, judged = "", false
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "x-email-presentation:") {
			judged = true
			continue
		}
		if match := propertyLine.FindStringSubmatch(line); match != nil {
			current = match[1]
			continue
		}
		match := refLine.FindStringSubmatch(line)
		if match == nil || current == "" || judged || !marked[match[1]] || seen[current] {
			continue
		}
		seen[current] = true
		fields = append(fields, current)
	}
	return fields
}

// consumersOf answers every frontend file that reaches this schema, by either
// route: naming its generated type, or reading a property that carries one.
//
// The camelCase spelling is searched alongside the contract's snake_case,
// because a file that maps the wire shape onto its own props keeps rendering
// the same message under the other spelling.
func consumersOf(t *testing.T, schema string, fields []string) []string {
	t.Helper()
	needle := fmt.Sprintf(`components["schemas"]["%s"]`, schema)
	// The field as a WHOLE word, in both spellings. Anchoring on the boundary
	// is what separates reading `entry.emailSummary` from calling
	// `emailSummaryText`, the shared preview splitter, whose name merely starts
	// with the field's — a census matching the bare prefix would report on a
	// function's spelling rather than on what the file does with a message.
	alternatives := make([]string, 0, len(fields)*2)
	for _, field := range fields {
		alternatives = append(alternatives, regexp.QuoteMeta(field), regexp.QuoteMeta(camelCase(field)))
	}
	access := regexp.MustCompile(`\b(` + strings.Join(alternatives, "|") + `)\b`)
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
		text := string(source)
		if !strings.Contains(text, needle) && !access.MatchString(text) {
			return nil
		}
		rel, relErr := filepath.Rel(emailFrontendSource, path)
		if relErr != nil {
			return relErr
		}
		found = append(found, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		t.Fatalf("walking the frontend sources: %v", err)
	}
	return found
}

// camelCase is the contract's snake_case as a TypeScript property.
func camelCase(field string) string {
	parts := strings.Split(field, "_")
	for i := 1; i < len(parts); i++ {
		if parts[i] == "" {
			continue
		}
		parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
	}
	return strings.Join(parts, "")
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
	fields := carrierFields(t, schemas)
	// The same floor for the other half of the corpus. With no carrier field
	// the census sees only the files that name a generated type, which is the
	// shape of the miss this gate was widened to catch.
	if len(fields) == 0 {
		t.Fatal("no property in the contract carries a schema marked " +
			"`x-email-presentation: entry` — this census can no longer see a surface " +
			"that reads an email off the record holding it")
	}
	// Reported once per file rather than once per schema it reaches: the two
	// entry schemas travel together on the same records, so a surface drawing
	// its own row is one defect with one fix, and naming it twice makes a
	// reader look for a second.
	reported := map[string]bool{}
	for _, schema := range schemas {
		for _, consumer := range consumersOf(t, schema, fields) {
			if emailRenderers[consumer] || describesRatherThanRenders(consumer) || reported[consumer] {
				continue
			}
			// Holding the data is not the offence; drawing it yourself is. A
			// surface that hands what it read to EmailEntry or EmailReference
			// has done the thing this gate asks for, and most of them must
			// touch the field to decide an email is what they have.
			if rendersCanonically(t, consumer) {
				continue
			}
			if emailPassthroughs.Waived(t, consumer) {
				continue
			}
			reported[consumer] = true
			t.Errorf("%s reads %s and draws it with its own markup — render EmailEntry "+
				"or EmailReference instead, or ratify the file in emailPassthroughs with "+
				"what it does with the type", consumer, schema)
		}
	}
}

// rendersCanonically is a file that mounts one of the canonical components.
//
// Matched as a JSX opening tag rather than as the bare word, so that naming a
// component in a comment or importing it and never mounting it does not
// discharge the obligation.
func rendersCanonically(t *testing.T, consumer string) bool {
	t.Helper()
	source, err := os.ReadFile(filepath.Join(emailFrontendSource, consumer))
	if err != nil {
		t.Fatalf("reading %s: %v", consumer, err)
	}
	text := string(source)
	for _, component := range []string{"EmailEntry", "EmailReference", "EmailDetail"} {
		if strings.Contains(text, "<"+component) {
			return true
		}
	}
	return false
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
	fields := carrierFields(t, schemas)
	drawing := map[string]bool{}
	for _, schema := range schemas {
		for _, consumer := range consumersOf(t, schema, fields) {
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

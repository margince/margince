// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// An identity never reaches a field a reader is shown.
//
// `cause_ref` groups repeated failures of one thing, and it has to be an
// IDENTITY to do that: a rule's name is mutable and not unique, so a cause built
// from names would merge two rules that happen to share one and split a rule
// somebody renamed. Its own contract says it is "never rendered".
//
// The client rendered it anyway, and a rep read "automation_run:01a05500-…
// failed 12 times". Fixing the client is half: the other half is making sure the
// server never puts a value of that shape somewhere the client is SUPPOSED to
// draw, because then every client is wrong and none of them can tell.
//
// This runs the real renderers over the awkward inputs rather than reading their
// source, so a renderer that starts composing its title out of a cause fails
// here even though the source never spells one.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// causeShaped matches the vocabulary the producers actually mint: a lowercase
// snake_case word, a colon, and something after it. Deliberately not "contains a
// colon" — a bounce reason may legitimately read "Rejected: no such mailbox",
// and a gate that failed on prose would be turned off rather than fixed.
var causeShaped = regexp.MustCompile(`\b[a-z][a-z0-9_]*:[^\s]`)

// renderedFields are the strings a client puts on screen. `cause_ref` is
// deliberately absent from this list — it is the identity, and carrying it is
// the whole point.
func renderedFields(item crmcontracts.AttentionItem) map[string]*string {
	return map[string]*string{
		"title":       item.Title,
		"detail":      item.Detail,
		"cause_label": item.CauseLabel,
	}
}

func causeRefRenderers(t *testing.T) map[string]crmcontracts.AttentionItem {
	t.Helper()
	at := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)
	return map[string]crmcontracts.AttentionItem{
		"capture_health": captureItem(CaptureConcern{
			ConnectionID: ids.NewV7(),
			Kind:         "disconnected",
			Provider:     "gmail",
			AccountLabel: "lena.fischer@margince.test",
		}),
		"ai_work_health": aiWorkItem(TroubledRun{
			ID:           ids.NewV7(),
			Kind:         "meeting_recap",
			State:        "failed",
			Summary:      "A recap did not generate",
			SubjectLabel: "Acme Renewal",
			OccurredAt:   at,
		}),
		"automation_run": automationItem(TroubledAutomationRun{
			ID:           ids.NewV7(),
			AutomationID: ids.New[ids.AutomationKind](),
			Name:         "Notify sales on a new lead",
			Outcome:      "failed",
			Reason:       "The webhook target answered 500 five times.",
			OccurredAt:   at,
		}),
	}
}

// The fixtures above are hand-written, because each renderer takes a different
// input and no scan can invent one. What CAN be derived is the census: which
// functions assign a CauseRef at all. A fourth producer added tomorrow joins
// this list by existing, and fails here until somebody gives it a fixture —
// which is the only way the tests above can be known to cover the tree.
//
// The failure mode this exists for is the quiet one: a new producer leaks an
// identity into a rendered field, and the gate reports PASS because it never
// heard of it.
func TestEveryCauseProducerHasAFixture(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the attention package directory: %v", err)
	}
	producers := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed, parseErr := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			t.Fatalf("parsing %s: %v", name, parseErr)
		}
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if assignsCauseRef(fn.Body) {
				producers[fn.Name.Name] = true
			}
		}
	}
	if len(producers) == 0 {
		t.Fatal("found no CauseRef producer in the package at all — the scan is looking in " +
			"the wrong place, and would report PASS over a tree where every one leaked")
	}
	// The fixture map is keyed by SOURCE; the census is keyed by function. Both
	// are small and the mapping is stated once, here, so a new producer fails
	// with a sentence saying what to do rather than a missing-key panic.
	fixtured := map[string]string{
		"captureItem":    "capture_health",
		"aiWorkItem":     "ai_work_health",
		"automationItem": "automation_run",
	}
	items := causeRefRenderers(t)
	for producer := range producers {
		source, known := fixtured[producer]
		if !known {
			t.Errorf("%s assigns a cause_ref and no fixture drives it, so the gates in this "+
				"file never judge what it renders. Add it to causeRefRenderers and to the "+
				"map above.", producer)
			continue
		}
		if _, built := items[source]; !built {
			t.Errorf("%s is mapped to the source %q, which causeRefRenderers does not build",
				producer, source)
		}
	}
}

// assignsCauseRef reports whether a function body assigns to a CauseRef field.
func assignsCauseRef(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, target := range assign.Lhs {
			if sel, ok := target.(*ast.SelectorExpr); ok && sel.Sel.Name == "CauseRef" {
				found = true
			}
		}
		return true
	})
	return found
}

// Every renderer that mints a cause, over inputs that fill each of its fields.
func TestNoRenderedFieldCarriesACauseIdentity(t *testing.T) {
	for source, item := range causeRefRenderers(t) {
		t.Run(source, func(t *testing.T) {
			// The renderer must actually have minted one, or this proves nothing:
			// a fixture that produced no cause would pass every assertion below
			// while testing a row this gate does not care about.
			if item.CauseRef == nil || *item.CauseRef == "" {
				t.Fatalf("%s minted no cause_ref for this input, so the case below is vacuous", source)
			}
			for name, value := range renderedFields(item) {
				if value == nil {
					continue
				}
				if causeShaped.MatchString(*value) {
					t.Errorf("%s.%s = %q, which reads like an identity. The contract says a "+
						"cause_ref is never rendered, and a reader shown one learns nothing "+
						"they can act on and cannot tell it from a bug.", source, name, *value)
				}
			}
		})
	}
}

// A label that IS sent must be words a reader reads.
//
// Not "must be sent". A lane with no reader-facing name for its condition mints
// none, and the group says what KIND of thing broke instead. The AI lane is
// exactly that: its only candidate is a generated task enum, and sending
// `site_triage` as the words would move the leak rather than close it.
//
// So this judges what a lane does send, in both directions a label goes wrong —
// the identity spelled twice, or a bare vocabulary key wearing the label's
// clothes. The second is the one that reads as a label until you look at it.
func TestAMintedLabelIsWordsAndNotVocabulary(t *testing.T) {
	// A key: lowercase snake_case, no spaces. A real label is a mailbox address
	// or a rule's name — something with a space, a dot or an at-sign in it.
	bareKey := regexp.MustCompile(`^[a-z][a-z0-9]*(_[a-z0-9]+)+$`)
	labelled := 0
	for source, item := range causeRefRenderers(t) {
		t.Run(source, func(t *testing.T) {
			if item.CauseLabel == nil || *item.CauseLabel == "" {
				return
			}
			labelled++
			label := *item.CauseLabel
			if label == *item.CauseRef {
				t.Errorf("%s carries the identity as its own label (%q): the two exist "+
					"because one groups and the other is read", source, label)
			}
			if bareKey.MatchString(label) {
				t.Errorf("%s's label %q is a vocabulary key rather than words. A reader "+
					"shown it learns as little as from the identity — a lane with no name "+
					"for its condition sends none, and the group says its kind instead.",
					source, label)
			}
		})
	}
	// Under-recognition guard: were every lane to stop labelling, the loop above
	// would judge nothing and still report PASS.
	if labelled == 0 {
		t.Fatal("no renderer minted a label at all, so this test judged nothing — either " +
			"the labels were dropped or the fixtures stopped producing them")
	}
}

// The gate's own reach, checked from the other end.
//
// A regex that matched nothing would report PASS over a tree where every
// renderer leaked, and nothing would fail to say so. So the pattern is asserted
// against the values these renderers really mint, and against the prose it must
// NOT fire on — a bounce reason with a colon in it is a sentence, and a gate
// that failed on those would be waived rather than fixed.
func TestTheIdentityPatternSeesRealCausesAndSparesRealProse(t *testing.T) {
	for source, item := range causeRefRenderers(t) {
		if !causeShaped.MatchString(*item.CauseRef) {
			t.Errorf("the pattern does not recognise %s's own cause %q — it would report "+
				"PASS over a renderer leaking exactly that shape", source, *item.CauseRef)
		}
	}
	prose := []string{
		"Rejected: no such mailbox at that domain.",
		"Held: the recipient has not consented to marketing mail.",
		"They asked to be introduced to the buyer at Turbinenbau.",
		"lena.fischer@margince.test",
		"Notify sales on a new lead",
	}
	for _, line := range prose {
		if causeShaped.MatchString(line) {
			t.Errorf("the pattern fires on the sentence %q — a gate that fails on prose "+
				"gets turned off rather than fixed", line)
		}
	}
	// A guard against the pattern being loosened into "anything with a colon":
	// the sentences above already cover that, and this names why they are there.
	if !strings.Contains(prose[0], ":") {
		t.Fatal("the prose corpus lost its colon case, which is the whole distinction")
	}
}

// A contact decision's supporting line, which used to be marker words.
//
// The queue groups these questions by two facts the verdict engine already
// decided, and those facts travelled as the words "machine_sender" and
// "known_company" inside `detail` — with a substring match to read them back.
// That is why this source's supporting line could not be drawn: any client
// rendering the field faithfully would have shown a rep an internal token.
//
// Now the facts are typed and the field is free to hold a sentence, or nothing.
// What it must never hold again is a marker.
func TestAContactDecisionsDetailCarriesNoMarkerWords(t *testing.T) {
	change := map[string]any{"email": "noreply@newsletter.example"}
	summary := "Is noreply@newsletter.example a contact worth keeping?"
	approval := crmcontracts.Approval{
		Id:             openapi_types.UUID(ids.NewV7()),
		Kind:           "capture_counterparty",
		Summary:        &summary,
		ProposedChange: &change,
	}

	item := approvalItem(approval, func(string) bool { return true })

	// The fact still reaches the queue — asserted first, so the absence below
	// cannot pass because nothing was computed at all.
	if item.Staged == nil || item.Staged.MachineSender == nil || !*item.Staged.MachineSender {
		t.Fatalf("the machine-sender fact did not reach the row: %+v", item.Staged)
	}
	if item.Detail != nil {
		t.Errorf("detail = %q on a contact decision, want none: this field is drawn to a "+
			"reader now, and a marker word here reaches them as %q", *item.Detail, *item.Detail)
	}
}

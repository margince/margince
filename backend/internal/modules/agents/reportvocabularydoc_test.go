// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// twoReportCatalog extends the package's one catalog fixture with the second
// shape a renderer can get wrong and probeReportCatalog cannot express: a report
// with NO measures, a threshold key sitting among the filters, and neither prose
// member set. Built on that fixture rather than beside it, so a change to what a
// catalog entry carries reaches both.
func twoReportCatalog() []ReportCatalogEntry {
	return append(slices.Clone(probeReportCatalog), ReportCatalogEntry{
		Report:  "projects-gone-quiet",
		GroupBy: []string{"phase"},
		// `days` is a THRESHOLD, and a catalog lists it among the filters
		// because `filters` is the object a caller sends it in.
		Filters:    []string{"days", "phase"},
		Aggregates: nil,
		Notes:      "`days` is a whole number of days of silence, default 30",
	})
}

// readDocument is what a client gets, decoded.
func readDocument(t *testing.T, catalog []ReportCatalogEntry) reportVocabularyDoc {
	t.Helper()
	body, err := NewReportVocabularyResource(catalog).ReportVocabularyDocument(context.Background())
	if err != nil {
		t.Fatalf("composing the document: %v", err)
	}
	var doc reportVocabularyDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		t.Fatalf("decoding the document: %v", err)
	}
	return doc
}

// Every vocabulary the catalog hands over reaches the document. This is the
// whole point of the move: the recital it replaced was what a caller read, so a
// name dropped on the way here is a name nothing on the surface yields.
func TestTheDocumentPublishesEveryVocabularyTheCatalogCarries(t *testing.T) {
	catalog := twoReportCatalog()
	doc := readDocument(t, catalog)
	if len(doc.Reports) != len(catalog) {
		t.Fatalf("the document publishes %d reports, the catalog holds %d", len(doc.Reports), len(catalog))
	}
	for i, entry := range catalog {
		published := doc.Reports[i]
		if published.Report != entry.Report {
			t.Fatalf("report %d is %q, want %q — the document reordered the catalog", i, published.Report, entry.Report)
		}
		for _, pair := range []struct {
			slot            string
			published, want []string
		}{
			{"group_by", published.GroupBy, entry.GroupBy},
			{"filters", published.Filters, entry.Filters},
			{"aggregates", published.Aggregates, entry.Aggregates},
		} {
			if len(pair.published) != len(pair.want) {
				t.Errorf("%s: %s publishes %v, the engine accepts %v",
					entry.Report, pair.slot, pair.published, pair.want)
				continue
			}
			for j, name := range pair.want {
				if pair.published[j] != name {
					t.Errorf("%s: %s[%d] is %q, want %q", entry.Report, pair.slot, j, pair.published[j], name)
				}
			}
		}
	}
	if doc.Reports[0].Defaults != catalog[0].Defaults {
		t.Errorf("defaults = %q, want %q — it is the call a caller should make first",
			doc.Reports[0].Defaults, catalog[0].Defaults)
	}
	if doc.Reports[0].Notes != catalog[0].Notes {
		t.Errorf("notes = %q, want %q", doc.Reports[0].Notes, catalog[0].Notes)
	}
}

// An empty vocabulary is published as `[]`, never as `null`.
//
// The two say different things: `[]` is a closed list holding nothing, `null`
// reads as a list not stated. A caller who reads an omission sends a plausible
// name into an argument that accepts none — which is why the recital this
// document replaced wrote `(none)` rather than nothing.
func TestAnEmptyVocabularyIsPublishedAsAnEmptyListNotNull(t *testing.T) {
	body, err := NewReportVocabularyResource(twoReportCatalog()).
		ReportVocabularyDocument(context.Background())
	if err != nil {
		t.Fatalf("composing the document: %v", err)
	}
	// Asserted on the BYTES, because the distinction is invisible after
	// decoding: both `[]` and `null` unmarshal into a nil-or-empty slice.
	if strings.Contains(string(body), `"aggregates":null`) {
		t.Errorf("a report with no measures publishes `null`, which reads as unstated:\n%s", body)
	}
	if !strings.Contains(string(body), `"aggregates":[]`) {
		t.Errorf("the report with no measures does not publish an empty list:\n%s", body)
	}
}

// The notation is stated, and it is the one thing a flat list of names cannot
// say: `filters` is a single object carrying two families of key. A caller who
// reads `days` beside `phase` and infers two separate arguments writes a plan
// that is refused for a shape it could not have seen.
func TestTheDocumentStatesHowFiltersAreSpelled(t *testing.T) {
	doc := readDocument(t, twoReportCatalog())
	if doc.Notation == "" {
		t.Fatal("the document states no notation, so `filters` holding thresholds is invisible")
	}
	if !strings.Contains(doc.Notation, "threshold") {
		t.Errorf("the notation never mentions thresholds, the one family a name list does not "+
			"distinguish: %s", doc.Notation)
	}
	if doc.Version == "" {
		t.Error("the document carries no version, so a client caching it cannot tell a shape change")
	}
}

// The provenance note is keyed on the vocabularies actually rendered. A catalog
// whose reports name no pipeline id must not carry advice about obtaining one:
// that was the recital's rule, and it is the document's now.
func TestTheProvenanceNoteIsKeyedOnWhatIsRendered(t *testing.T) {
	withPipeline := readDocument(t, twoReportCatalog())
	if len(withPipeline.Notes) == 0 {
		t.Fatal("a catalog naming pipeline_id carries no note about where one comes from, so a " +
			"correct refusal dead-ends")
	}
	if !strings.Contains(withPipeline.Notes[0], "list_pipelines") {
		t.Errorf("the note does not name the tool that yields the id: %s", withPipeline.Notes[0])
	}
	// The same catalog with both ids gone from every slot of every report — the
	// note is keyed on all three vocabularies, so clearing one would leave the
	// case passing for the wrong reason.
	noPipeline := twoReportCatalog()
	noPipeline[0].GroupBy = []string{"status"}
	noPipeline[0].Filters = []string{"owner_id"}
	if notes := readDocument(t, noPipeline).Notes; len(notes) != 0 {
		t.Errorf("a catalog naming no pipeline id still carries %v — advice about a key no plan "+
			"can use", notes)
	}
}

// The two doors serve ONE document. The resource and the tool both read
// ReportVocabularyDocument, so this holds the seam rather than the agreement:
// two renderings that agree today are the second copy the move removed.
func TestTheResourceAndTheSeamServeTheSameBytes(t *testing.T) {
	resource := NewReportVocabularyResource(twoReportCatalog())
	seam, err := resource.ReportVocabularyDocument(context.Background())
	if err != nil {
		t.Fatalf("the seam: %v", err)
	}
	contents, err := resource.ReadResource(context.Background(), ReportVocabularyURI)
	if err != nil {
		t.Fatalf("the resource: %v", err)
	}
	if contents.Text != string(seam) {
		t.Errorf("the resource serves\n%s\nand the seam serves\n%s", contents.Text, seam)
	}
	if contents.MIMEType != mimeApplicationJSON {
		t.Errorf("MIMEType = %q, want %q", contents.MIMEType, mimeApplicationJSON)
	}
}

// Byte-stable across compositions. A document that reshuffles per boot reads as
// a changed resource to a client that caches it, and would make a client
// re-fetch a document that never changed.
func TestTheDocumentIsByteStable(t *testing.T) {
	first, err := NewReportVocabularyResource(twoReportCatalog()).
		ReportVocabularyDocument(context.Background())
	if err != nil {
		t.Fatalf("composing the document: %v", err)
	}
	for range 8 {
		again, err := NewReportVocabularyResource(twoReportCatalog()).
			ReportVocabularyDocument(context.Background())
		if err != nil {
			t.Fatalf("composing the document: %v", err)
		}
		if string(again) != string(first) {
			t.Fatalf("two compositions of one catalog differ:\n%s\n%s", first, again)
		}
	}
}

// An unknown URI is not found, matching every other read on this surface — and
// not answered with this document, which would serve the report vocabulary
// under whatever URI a caller happened to ask for.
func TestAnUnknownURIIsNotThisDocument(t *testing.T) {
	_, err := NewReportVocabularyResource(twoReportCatalog()).
		ReadResource(context.Background(), "margince://schema/record-fields")
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// The catalogue entry says what the document HOLDS and never orders a read.
//
// This sentence rides the Surface-B prompt in full. As an instruction — "read
// this before planning a report" — it drew reads from runs with no report in
// them, and the two write tools stopped doing it for that measured reason.
func TestTheCatalogueEntryDoesNotOrderARead(t *testing.T) {
	published := NewReportVocabularyResource(twoReportCatalog()).Resources(context.Background())
	if len(published) != 1 {
		t.Fatalf("the provider publishes %d resources, want exactly 1", len(published))
	}
	entry := published[0]
	if entry.URI != ReportVocabularyURI {
		t.Errorf("URI = %q, want %q", entry.URI, ReportVocabularyURI)
	}
	if entry.RequiredScope != principal.ScopeRead {
		t.Errorf("RequiredScope = %v, want read — the same grant run_report spends", entry.RequiredScope)
	}
	for _, imperative := range []string{"read this", "read it", "before your first", "you must read"} {
		if strings.Contains(strings.ToLower(entry.Description), imperative) {
			t.Errorf("the description orders a read (%q), which is measured to draw reads from "+
				"runs that had no use for one: %s", imperative, entry.Description)
		}
	}
	if !strings.Contains(entry.Description, "run_report") {
		t.Error("the description does not name the tool that consumes it, so a caller learns " +
			"the vocabulary without learning what to do with it")
	}
}

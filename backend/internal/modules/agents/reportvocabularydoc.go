// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// `margince://schema/reports` — the three plan vocabularies each prebuilt report
// accepts, published as a document rather than recited into run_report's own
// input schema.
//
// It used to be recited. run_report's `report` property carried, for all nine
// prebuilt reports, their group_by, filters, aggregates, default answer and
// notes — 3.4KB of one tool's description, making run_report 6% of the whole
// served catalog and 2.3× the next tool. Text every client holds for the whole
// session, and every Surface-B run re-sends on every step, to answer a question
// one call asks once.
//
// Moving it is the same move the write vocabulary made to
// margince://schema/record-fields, and it is not the schema deferral this
// surface rules out. Every argument that decides whether a call is WELL-FORMED
// stays in the schema — `report`'s enum included, which closes the first of a
// caller's two questions at zero round trips. What moves is the per-report
// vocabulary, and it moves because the refusal path is loud: a name outside a
// report's list is refused BY NAME with that slot's accepted list
// (FieldNotAllowedError), and a key outside the catalog is refused with every
// key that exists (UnknownReportError).

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// ReportVocabularyURI is the document's stable identity.
const ReportVocabularyURI = "margince://schema/reports"

// reportVocabularyResourceName is the catalogue entry's name member.
const reportVocabularyResourceName = "report_vocabulary"

// reportVocabularyVersion identifies the document's SHAPE, not its content. A
// caller caching it needs to know when the shape changed; the vocabularies move
// with the engine's prebuilt catalog on their own schedule and are re-read
// either way.
const reportVocabularyVersion = "1"

// reportVocabularyNotation states the one convention the entries are written in,
// because `filters` is the member a reader is most likely to get wrong: it is a
// single object carrying two families of key, and nothing in a flat list of
// names says so.
const reportVocabularyNotation = "Each report accepts ONLY the names listed under it. `group_by` and " +
	"`aggregates` take names from their own lists; `filters` is one object whose keys come from " +
	"the `filters` list, which holds both equality predicates and numeric thresholds. A name " +
	"outside a report's list is refused by name, with that list, rather than approximated."

// ReportVocabularyResource publishes the report plan vocabularies.
//
// It holds the catalog rather than deriving one: the prebuilt reports are where
// the SQL lives, so the composition root hands the same []ReportCatalogEntry
// here that it hands RegisterReportTool. Two derivations of one catalog is the
// second copy this move exists to remove.
type ReportVocabularyResource struct{ catalog []ReportCatalogEntry }

// NewReportVocabularyResource binds the document to the catalog it describes.
func NewReportVocabularyResource(catalog []ReportCatalogEntry) ReportVocabularyResource {
	return ReportVocabularyResource{catalog: catalog}
}

// Resources advertises the one document this provider publishes.
func (ReportVocabularyResource) Resources(context.Context) []mcp.Resource {
	return []mcp.Resource{{
		URI:   ReportVocabularyURI,
		Name:  reportVocabularyResourceName,
		Title: "Report plan vocabulary",
		// ScopeRead, matching run_report: a caller admitted to the tool is
		// admitted to the vocabulary it refuses against, by the same grant.
		RequiredScope: principal.ScopeRead,
		MIMEType:      mimeApplicationJSON,
		// Says what it HOLDS, not when to fetch it. A description that orders a
		// read is measured to draw reads from runs with nothing to plan, which
		// is why the two write tools stopped doing it.
		Description: "What each prebuilt report accepts in a run_report plan: the names its " +
			"group_by, filters and aggregates admit, what it answers with no plan at all, and " +
			"what a filter means when its name alone does not say. run_report names this " +
			"document instead of carrying it.",
	}}
}

// ReadResource composes the document. An unknown URI answers ErrNotFound,
// matching every other read on this surface.
func (r ReportVocabularyResource) ReadResource(ctx context.Context, uri string) (mcp.ResourceContents, error) {
	if uri != ReportVocabularyURI {
		return mcp.ResourceContents{}, fmt.Errorf("agents: resource %q: %w", uri, apperrors.ErrNotFound)
	}
	// Through the same seam the tool reads, so the two doors serve one byte
	// sequence rather than two renderings that agree today.
	body, err := r.ReportVocabularyDocument(ctx)
	if err != nil {
		return mcp.ResourceContents{}, err
	}
	return mcp.ResourceContents{URI: uri, MIMEType: mimeApplicationJSON, Text: string(body)}, nil
}

// ReportVocabularyDocument is the ONE composition, read by the resource above
// and by describe_report_vocabulary — so the two doors cannot drift into two
// answers to one question.
//
// The context is unused here: the document is the engine's compile-time catalog
// and the same for every caller. It is in the signature because the seam's other
// implementation is the overlay guard, which reads the workspace's mode.
func (r ReportVocabularyResource) ReportVocabularyDocument(context.Context) (json.RawMessage, error) {
	body, err := json.Marshal(r.document())
	if err != nil {
		return nil, fmt.Errorf("agents: rendering the report plan vocabulary: %w", err)
	}
	return body, nil
}

var (
	_ mcp.ResourceProvider   = ReportVocabularyResource{}
	_ ReportVocabularyReader = ReportVocabularyResource{}
)

// reportVocabularyDoc is the published shape.
type reportVocabularyDoc struct {
	Version  string                  `json:"version"`
	Notation string                  `json:"notation"`
	Reports  []reportVocabularyEntry `json:"reports"`
	// Notes carries what is true of the SURFACE rather than of one report —
	// today, where the ids several reports key on come from. Keyed on the
	// vocabularies actually rendered, so a catalog without those keys does not
	// carry advice about them.
	Notes []string `json:"notes,omitempty"`
}

// reportVocabularyEntry is one report as a plan author sees it. Named members
// rather than the catalog type itself: this is a wire shape with JSON tags a
// client reads, and ReportCatalogEntry is the seam the engine hands over.
type reportVocabularyEntry struct {
	Report     string   `json:"report"`
	GroupBy    []string `json:"group_by"`
	Filters    []string `json:"filters"`
	Aggregates []string `json:"aggregates"`
	// Defaults says what the report answers when the plan arguments are
	// omitted, which is the call a caller should make first.
	Defaults string `json:"defaults,omitempty"`
	// Notes says what a name MEANS when the name alone does not: which rows
	// `project_id` admits, what unit `days` is in.
	Notes string `json:"notes,omitempty"`
}

// document renders the catalog in the order it arrives, which
// ReportCatalogEntry documents as sorted by its producer, entry by entry. Not
// re-sorted here: a second sort would be a second copy of that guarantee, and
// the description renderer next door has always relied on the same one. The
// stability matters because a document that reshuffles per boot reads as a
// changed resource to a client that caches it.
func (r ReportVocabularyResource) document() reportVocabularyDoc {
	entries := make([]reportVocabularyEntry, 0, len(r.catalog))
	for _, entry := range r.catalog {
		entries = append(entries, reportVocabularyEntry{
			Report:     entry.Report,
			GroupBy:    closedList(entry.GroupBy),
			Filters:    closedList(entry.Filters),
			Aggregates: closedList(entry.Aggregates),
			Defaults:   entry.Defaults,
			Notes:      entry.Notes,
		})
	}
	return reportVocabularyDoc{
		Version:  reportVocabularyVersion,
		Notation: reportVocabularyNotation,
		Reports:  entries,
		Notes:    reportVocabularySurfaceNotes(r.catalog),
	}
}

// closedList renders an empty vocabulary as `[]` and never as `null`, because
// the two say different things to a reader: `[]` is a closed list with nothing
// in it, and `null` reads as a list not stated — an omission. Two reports
// declare no measures at all, and a caller who reads that as an omission sends
// a plausible aggregate name into an argument that accepts none. The recital
// this document replaced wrote `(none)` for exactly this reason.
func closedList(names []string) []string {
	if names == nil {
		return []string{}
	}
	return names
}

// reportVocabularySurfaceNotes closes the obligation the vocabularies open:
// several reports filter and group by `pipeline_id` and `stage_id`, and naming
// an id a caller cannot obtain is a correct refusal that dead-ends.
func reportVocabularySurfaceNotes(catalog []ReportCatalogEntry) []string {
	for _, entry := range catalog {
		for _, names := range [][]string{entry.GroupBy, entry.Filters, entry.Aggregates} {
			if slices.Contains(names, "pipeline_id") || slices.Contains(names, "stage_id") {
				return []string{"A `pipeline_id` or `stage_id` used in a plan comes from list_pipelines."}
			}
		}
	}
	return nil
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reportdoc

// The grammar, described for a caller that has to write one.
//
// Derived from the same predicates the validator refuses with — Kind.known(),
// Kind.carriesFigures(), Severity.known() — rather than typed out beside them.
// A hand-written list here would be a second copy of the grammar, and the two
// would disagree the first time a block was added: the document would name a
// kind the validator refuses, or omit one it accepts.

// BlockDescription is one kind, as a composer needs to know it.
type BlockDescription struct {
	Kind string `json:"kind"`
	// TakesCells says the kind renders figures, and must name at least one
	// cell. False means a cell on it is refused: the figure would be composed
	// and never shown.
	TakesCells bool `json:"takes_cells"`
	// TakesText says the kind renders the composer's own words.
	TakesText bool `json:"takes_text"`
	// TakesSeverity is true only for a callout, which states its kind.
	TakesSeverity bool `json:"takes_severity"`
}

// Catalog describes every block a document may carry.
//
// Iterated over the declared order rather than a map, so two calls return one
// document and a caller diffing them sees nothing move.
func Catalog() []BlockDescription {
	out := make([]BlockDescription, 0, len(allKinds))
	for _, k := range allKinds {
		out = append(out, BlockDescription{
			Kind:          string(k),
			TakesCells:    k.carriesFigures() || k == KindSummary || k == KindEvidenceDrawer,
			TakesText:     !k.carriesFigures(),
			TakesSeverity: k == KindCallout,
		})
	}
	return out
}

// Severities lists the closed severity set a callout may state.
func Severities() []string {
	out := make([]string, 0, len(allSeverities))
	for _, s := range allSeverities {
		out = append(out, string(s))
	}
	return out
}

// allKinds is the declared order the catalog is published in — prose first,
// then figures, then what a reader is owed when a figure is incomplete.
//
// Kind.known() is derived FROM this rather than switching separately, so a kind
// added here is accepted by the validator and described by the catalog in one
// edit. The alternative — a switch beside a list — is the two-copies defect
// this file exists to avoid.
var allKinds = []Kind{
	KindTitle, KindSubtitle, KindScope, KindGeneratedAt, KindSummary,
	KindMethodology, KindFollowUps,
	KindStatStrip, KindBar, KindWaterfall, KindRankedList, KindRecordTable,
	KindCallout, KindEvidenceDrawer,
}

// allSeverities is the closed set, in the order a reader meets them.
var allSeverities = []Severity{
	SeverityNote, SeverityWarning, SeverityPartial, SeverityUnknown, SeverityUnsupported,
}

// KindNames lists every accepted kind, for a refusal that names the whole set.
func KindNames() []string {
	out := make([]string, 0, len(allKinds))
	for _, k := range allKinds {
		out = append(out, string(k))
	}
	return out
}

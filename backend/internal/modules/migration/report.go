// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

// What a run reports, and how two attempts at one run become one report.
//
// Separate from the engine because the folding is the part with a rule in it:
// row counts add because the checkpoint guarantees no row is walked twice,
// edges REPLACE because the association phase has no checkpoint and runs whole
// every time, and attempts count because nothing else in a stored report can
// tell one walk from five.

// SkippedRow is one disclosed skip in the run report.
type SkippedRow struct {
	ExternalID string `json:"external_id"`
	// Line is the file line the row came from, so the person reading the report
	// can go to it. Zero when the source has no file behind it.
	Line   int    `json:"line,omitempty"`
	Reason string `json:"reason"`
}

// skipReasonEmptyPayload marks a source row with no fields at all — the
// "payload-less system entries" class the parity preview must disclose
// rather than silently drop (UC-E18-04 E2).
const skipReasonEmptyPayload = "empty_payload"

// ObjectReport is one object class's slice of the run/dry-run report.
type ObjectReport struct {
	Object      string `json:"object"`
	MirrorCount int    `json:"mirror_count"`
	WillCreate  int    `json:"will_create"`
	WillUpdate  int    `json:"will_update"`
	Created     int    `json:"created"`
	Updated     int    `json:"updated"`
	// Unchanged counts rows already landed under their provenance that
	// this attempt did not rewrite (a resumed run's replayed page).
	Unchanged int          `json:"unchanged,omitempty"`
	Skipped   []SkippedRow `json:"skipped,omitempty"`
	// Collisions are rows that WILL land and also name a record the estate
	// already holds. They are NOT skips: each is counted in Created, and the
	// disposition's four counts must keep summing to the rows read. The
	// warning exists so a person approving "create 3" is not surprised by a
	// duplicate afterwards.
	Collisions []SkippedRow `json:"collisions,omitempty"`
	// WillDuplicate and Duplicated count rows naming a record the estate
	// already holds: what the PREVIEW predicted, and what the COMMIT observed.
	// Both overlap the other counts by design — each is also in Created or in
	// Skipped — so neither is ever added to them.
	//
	// Two fields for the reason WillCreate and Created are two fields, and the
	// merge is why it has to be. A run's stored report is the dry run's folded
	// together with every commit attempt's, and the two walk the SAME rows: one
	// field would report a file with one duplicate as having two the moment it
	// was approved. Each attempt writes only its own, so addition stays honest
	// across a resume, where the checkpoint does guarantee disjoint rows.
	//
	// WillDuplicate keeps the `duplicates` tag it was written under. The name
	// is what changed, not the field: a report stored before the commit learned
	// to count still decodes into the half it actually described.
	WillDuplicate int      `json:"duplicates,omitempty"`
	Duplicated    int      `json:"duplicated,omitempty"`
	Disclosures   []string `json:"disclosures,omitempty"`
}

// Report is the run (or dry-run) outcome: per-object dispositions plus
// the association tally. The disposition table is the honest-disclosure
// surface — nothing is dropped without a line here.
type Report struct {
	Objects []ObjectReport `json:"objects"`
	// Associations counts edges APPLIED, not edges read; AssociationsSkipped
	// carries the rest with their reasons.
	Associations        int            `json:"associations"`
	AssociationsSkipped []SkippedAssoc `json:"associations_skipped,omitempty"`
	Imported            int64          `json:"imported"`
	// Attempts is how many times this run has been walked, counting the dry
	// run. Zero and one both mean once — a report written before this was
	// carried says nothing, and saying nothing is the single-attempt case.
	//
	// It is here because attempts fold by object CLASS, so nothing else in the
	// stored report can count them: a CSV import is always exactly one class,
	// and the object count a reader reached for is 1 whether the run was walked
	// once or five times.
	Attempts int `json:"attempts,omitempty"`
}

// Walks is Attempts read as a count, where an unset field means the one walk
// that wrote it.
func (r Report) Walks() int {
	if r.Attempts < 1 {
		return 1
	}
	return r.Attempts
}

// SkippedAssoc is one edge the import did not materialize, disclosed.
type SkippedAssoc struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Reason string `json:"reason"`
}

// record folds one row's outcome into the class's disposition.
func (or *ObjectReport) record(row Row, res EnsureResult) {
	switch {
	case res.Skipped:
		or.Skipped = append(or.Skipped, SkippedRow{ExternalID: row.ExternalID, Line: row.Line, Reason: res.SkipReason})
	case res.Unchanged:
		or.Unchanged++
	case res.Created:
		or.Created++
	default:
		or.Updated++
	}
	if res.Duplicate {
		// Counted beside the outcome rather than instead of it: this row is
		// already in Created or in Skipped above, and the four load-bearing
		// counts must keep summing to the rows read.
		or.Duplicated++
	}
	if res.Disclosure != "" {
		or.Disclosures = append(or.Disclosures, res.Disclosure)
	}
}

// withPartial is the report as it stands plus the class currently being
// imported — what an attempt has actually landed at the moment it dies.
// The object is appended rather than merged: the run loop only appends a
// class once it finishes, so a crashed class has no entry yet.
func withPartial(rep Report, or ObjectReport) Report {
	rep.Objects = append(append([]ObjectReport(nil), rep.Objects...), or)
	rep.Imported += int64(or.Created + or.Updated)
	return rep
}

// mergedWith folds an earlier attempt's dispositions into this one's, so
// a run resumed across several crashes still reports the whole estate it
// imported rather than only its final leg. Row counts add and per-object
// entries fold by class; the checkpoint guarantees no row is walked
// twice, so addition cannot double-count them.
//
// EDGES REPLACE RATHER THAN ADD, and the difference is the checkpoint. There is
// none over the association phase: it runs whole on every attempt, and the dry
// run walks the same edges again to say what it would link. Adding those would
// report a file's 12,000 employer links as 24,000 the moment it was approved,
// and name every company it could not find twice over. The later answer is the
// true one — it was computed against the estate as it stands now — so it wins
// outright.
func (r Report) mergedWith(next Report) Report {
	// The later attempt's edges win — unless it never reached them. A resume
	// that failed before the association phase reports zero applied and zero
	// skipped, and taking that as the answer would erase what an earlier
	// attempt actually linked and report a run that connected nothing. Two
	// zeroes are indistinguishable from a phase that ran and found no edges, so
	// keeping the earlier answer is right either way: there the earlier is zero
	// too.
	associations, associationsSkipped := next.Associations, next.AssociationsSkipped
	if next.Associations == 0 && len(next.AssociationsSkipped) == 0 {
		associations, associationsSkipped = r.Associations, r.AssociationsSkipped
	}
	out := Report{
		Associations:        associations,
		AssociationsSkipped: append([]SkippedAssoc(nil), associationsSkipped...),
		Imported:            r.Imported + next.Imported,
		Attempts:            r.Walks() + next.Walks(),
	}
	at := map[string]int{}
	for _, or := range append(append([]ObjectReport(nil), r.Objects...), next.Objects...) {
		i, seen := at[or.Object]
		if !seen {
			at[or.Object] = len(out.Objects)
			out.Objects = append(out.Objects, or)
			continue
		}
		into := &out.Objects[i]
		into.Created += or.Created
		into.Updated += or.Updated
		into.Unchanged += or.Unchanged
		into.WillCreate += or.WillCreate
		into.WillUpdate += or.WillUpdate
		// MirrorCount is the source's size, not a tally: both attempts saw
		// the same frozen snapshot, so the later read stands.
		into.MirrorCount = or.MirrorCount
		into.Skipped = append(into.Skipped, or.Skipped...)
		into.Collisions = append(into.Collisions, or.Collisions...)
		into.WillDuplicate += or.WillDuplicate
		into.Duplicated += or.Duplicated
		into.Disclosures = append(into.Disclosures, or.Disclosures...)
	}
	return out
}

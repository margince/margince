// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What an alert can see.
//
// Before this family the only monitorable signal was the discarded count going
// up, which rises identically for a provider outage, a revoked credential and a
// bug. These hold the three properties that make the class worth publishing: it
// is the SAME class the screen shows, a failure nobody classified still counts,
// and the number of series stays inside a bound somebody stated.

import (
	"bytes"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/jobs"
)

// renderFailures answers the exposition for one set of failure groups.
func renderFailures(t *testing.T, failures ...jobs.FailureCount) string {
	t.Helper()
	var buf bytes.Buffer
	if err := writeJobFailureGauge(&buf, failures); err != nil {
		t.Fatalf("writeJobFailureGauge: %v", err)
	}
	return buf.String()
}

// The class an alert matches has to be the class the screen shows, or an
// operator paged by one goes looking for the other.
func TestAFailureIsPublishedUnderTheClassTheScreenShows(t *testing.T) {
	// A core sentinel's own sentence, read back through the same table the
	// failure list reads. Taken from the vocabulary rather than typed here:
	// a literal would pass while the sentence it copies changed underneath.
	detail, ok := jobs.VettedFailure("any_kind", jobs.SentenceForClass("record_gone"))
	if !ok {
		t.Fatal("the core vocabulary no longer answers for record_gone, so this case is testing nothing")
	}
	got := renderFailures(t, jobs.FailureCount{Kind: "person_enrich", Stored: detail.Sentence, Count: 3})
	want := `margince_job_failures{kind="person_enrich",class="record_gone"} 3`
	if !strings.Contains(got, want) {
		t.Errorf("the exposition does not publish the screen's own class\nwant: %s\ngot:\n%s", want, got)
	}
}

// A failure in a shape nobody enumerated is exactly the one an outage arrives
// in. Dropping it would make that outage invisible to the alert as well as to
// the screen — the surface going quiet in the one case it exists for.
func TestAFailureNobodyClassifiedStillCounts(t *testing.T) {
	got := renderFailures(t,
		jobs.FailureCount{Kind: "person_enrich", Stored: "dial tcp 10.0.0.1:443: i/o timeout", Count: 2},
		// And a row that recorded no cause at all, which lands on the same
		// class: the difference matters to somebody reading one failure and
		// not to a count of how many are failing unnamed.
		jobs.FailureCount{Kind: "person_enrich", Stored: "", Count: 1},
	)
	want := `margince_job_failures{kind="person_enrich",class="` + unclassifiedFailureClass + `"} 3`
	if !strings.Contains(got, want) {
		t.Errorf("an unrecognised failure was not counted\nwant: %s\ngot:\n%s", want, got)
	}
}

// One series per (kind, class), summed — not one per stored sentence. Two
// sentences of one class are one fact to an alert, and publishing them apart
// would make the class label mean nothing.
func TestTwoSentencesOfOneClassAreOneSeries(t *testing.T) {
	got := renderFailures(t,
		jobs.FailureCount{Kind: "person_enrich", Stored: "one unrecognised thing", Count: 2},
		jobs.FailureCount{Kind: "person_enrich", Stored: "another unrecognised thing", Count: 5},
	)
	if strings.Count(got, "margince_job_failures{kind=") != 1 {
		t.Errorf("want one series for one kind and class, got:\n%s", got)
	}
	if !strings.Contains(got, `class="`+unclassifiedFailureClass+`"} 7`) {
		t.Errorf("the two sentences did not sum onto one series:\n%s", got)
	}
}

// The exposition's cardinality is meant to be KNOWABLE, not small — and
// nothing was checking this family's. Both halves of the product grow when
// somebody adds a composed unit, which is the growth this turns from a
// surprise into a number a reviewer sees.
func TestTheFailureSeriesBoundIsWhatTheVocabulariesAllow(t *testing.T) {
	kinds := []string{"person_enrich", "company_enrich"}
	var failures []jobs.FailureCount
	for _, kind := range kinds {
		for _, class := range jobs.CoreFailureClasses() {
			failures = append(failures,
				jobs.FailureCount{Kind: kind, Stored: jobs.SentenceForClass(class), Count: 1})
		}
		failures = append(failures,
			jobs.FailureCount{Kind: kind, Stored: "nothing recognises this", Count: 1})
	}
	got := strings.Count(renderFailures(t, failures...), "margince_job_failures{kind=")
	if bound := failureSeriesBound(len(kinds)); got > bound {
		t.Errorf("the exposition published %d series against a stated bound of %d — "+
			"a cardinality nobody can count is a cardinality nobody is keeping", got, bound)
	}
	// And the bound is not vacuous: every core class plus the reserved one,
	// for each kind, is what was rendered.
	if want := len(kinds) * (len(jobs.CoreFailureClasses()) + 1); got != want {
		t.Errorf("rendered %d series, want %d — the case is no longer exercising the whole vocabulary", got, want)
	}
}

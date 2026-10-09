// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// What the weekly message says about a figure its source never measured. The
// Home panel says "Not recorded" for these, and a message printing 0 beside it
// would tell the rep their week was empty on no evidence.

import (
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

func coverageOf(status crmcontracts.WeeklyFigureCoverageStatus, since *time.Time) *crmcontracts.WeeklyFigureCoverage {
	return &crmcontracts.WeeklyFigureCoverage{Status: status, RecordedSince: since}
}

// coveredFixture is mailFixture frozen with coverage, in a zone east of UTC so
// a date read without the zone lands on the wrong day.
func coveredFixture(set crmcontracts.WeeklyFigureCoverageSet) Review {
	review := mailFixture()
	review.NumericSummary = &crmcontracts.WeeklyNumericSummary{
		Timezone: "Europe/Berlin", BookingsCoverage: readBookings, FigureCoverage: &set,
	}
	return review
}

// readBookings is a bookings read that saw everything, so Won is a measurement.
var readBookings = crmcontracts.ReportingCoverage{Status: crmcontracts.ReportingStatusOk}

func everyFamilyNotRecorded(since time.Time) crmcontracts.WeeklyFigureCoverageSet {
	notRecorded := coverageOf(crmcontracts.WeeklyFigureCoverageStatusNotRecorded, &since)
	return crmcontracts.WeeklyFigureCoverageSet{
		Deals: notRecorded, Tasks: notRecorded, Meetings: notRecorded, Leads: notRecorded, Commitments: notRecorded,
	}
}

// rowOf is the body line a tally label opens. A missing row fails the test, so
// no assertion on a row can pass because the row is gone.
func rowOf(t *testing.T, body, label string) string {
	t.Helper()
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, label) {
			return line
		}
	}
	t.Fatalf("the message has no %q row:\n%s", label, body)
	return ""
}

// 22:30 UTC on the 14th is the 15th in Berlin, which is the date the panel
// prints for a Berlin installation.
var firstRecord = time.Date(2026, 6, 14, 22, 30, 0, 0, time.UTC)

func TestAWeekBeforeEverySourceSaysSoAndPrintsNoZero(t *testing.T) {
	body := MailBody(coveredFixture(everyFamilyNotRecorded(firstRecord)), "", english)

	for _, want := range []string{
		"This week falls before the first records, so it has no figures to report.",
		"Not recorded · Recorded from 2026-06-15",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the message does not say %q:\n%s", want, body)
		}
	}
	for _, label := range []string{"Tasks delivered:", "Won · Lost · Moved:", "Carried over:"} {
		if row := rowOf(t, body, label); !strings.HasSuffix(row, "Not recorded · Recorded from 2026-06-15") {
			t.Errorf("the %q row prints a figure its source never measured: %q", label, row)
		}
	}
	// Proposals and the morning queue carry no coverage, so their counts stand.
	if !strings.Contains(body, "4 yes · 2 no") {
		t.Errorf("the message dropped a figure that has no coverage to qualify it:\n%s", body)
	}
}

// A family the review states nothing about counts as measured, as on the panel.
// A seat without lead read is never told its week predates the records.
func TestAWeekWithAnUnstatedFamilyIsNotCalledBeforeHistory(t *testing.T) {
	set := everyFamilyNotRecorded(firstRecord)
	set.Leads = nil

	body := MailBody(coveredFixture(set), "", english)

	if strings.Contains(body, "falls before the first records") {
		t.Errorf("an unstated family still produced the before-history line:\n%s", body)
	}
	if !strings.Contains(body, "Not recorded") {
		t.Errorf("the unrecorded figures lost their own statement:\n%s", body)
	}
}

func TestAPartialWeekKeepsItsCountAndSaysWhenCountingBegan(t *testing.T) {
	recorded := coverageOf(crmcontracts.WeeklyFigureCoverageStatusRecorded, &firstRecord)
	partial := coverageOf(crmcontracts.WeeklyFigureCoverageStatusPartial, &firstRecord)

	body := MailBody(coveredFixture(crmcontracts.WeeklyFigureCoverageSet{Deals: partial, Tasks: recorded}), "", english)

	if row := rowOf(t, body, "Won · Lost · Moved:"); !strings.HasSuffix(row, "1 · 1 · 3 · Partial week: counted from 2026-06-15") {
		t.Errorf("the partial deals figure is not qualified with its start: %q", row)
	}
	if row := rowOf(t, body, "Tasks delivered:"); !strings.HasSuffix(row, " 7 of 9") {
		t.Errorf("a fully recorded figure gained a qualifier: %q", row)
	}
}

func TestAReviewFrozenBeforeCoverageReadsAsItAlwaysDid(t *testing.T) {
	review := mailFixture()
	review.NumericSummary = &crmcontracts.WeeklyNumericSummary{Timezone: "Europe/Berlin", BookingsCoverage: readBookings}

	if got, want := MailBody(review, "", english), MailBody(mailFixture(), "", english); got != want {
		t.Errorf("a review with no coverage renders differently:\n%s\nwant:\n%s", got, want)
	}
}

func TestTheCoverageWordsAreTheInstallationsLanguage(t *testing.T) {
	body := MailBody(coveredFixture(everyFamilyNotRecorded(firstRecord)), "", mailcopy.For(string(mailcopy.German)))

	for _, want := range []string{
		"Diese Woche liegt vor den ersten Datensätzen",
		"Nicht erfasst · Erfasst ab 2026-06-15",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the German message does not say %q:\n%s", want, body)
		}
	}
}

func TestWonReadsUnavailableWhenItsBookingsWereNotRead(t *testing.T) {
	for name, bookings := range map[string]crmcontracts.ReportingCoverage{
		"withheld":    {Status: crmcontracts.ReportingStatusOk, Withheld: true},
		"unsupported": {Status: crmcontracts.ReportingStatusUnsupported},
	} {
		t.Run(name, func(t *testing.T) {
			review := coveredFixture(crmcontracts.WeeklyFigureCoverageSet{})
			review.NumericSummary.BookingsCoverage = bookings

			body := MailBody(review, "", english)

			if row := rowOf(t, body, "Won · Lost · Moved:"); !strings.HasSuffix(row, " Unavailable · 1 · 3") {
				t.Errorf("Won prints a count its bookings read never saw: %q", row)
			}
		})
	}
}

// A partial bookings read marks Won itself; lost and moved, counted from the
// deals alone, stay as they are.
func TestAPartialBookingsReadMarksWonAlone(t *testing.T) {
	review := coveredFixture(crmcontracts.WeeklyFigureCoverageSet{})
	review.NumericSummary.BookingsCoverage = crmcontracts.ReportingCoverage{Status: crmcontracts.ReportingStatusPartial}

	body := MailBody(review, "", english)

	if row := rowOf(t, body, "Won · Lost · Moved:"); !strings.HasSuffix(row, " 1 (partial) · 1 · 3") {
		t.Errorf("a partial bookings read did not mark Won: %q", row)
	}
}

// A deals family that has not begun outranks the bookings read, as on the
// panel. The week predating the source is why nothing could be read.
func TestNotRecordedOutranksUnavailable(t *testing.T) {
	review := coveredFixture(everyFamilyNotRecorded(firstRecord))
	review.NumericSummary.BookingsCoverage = crmcontracts.ReportingCoverage{Status: crmcontracts.ReportingStatusUnavailable}

	row := rowOf(t, MailBody(review, "", english), "Won · Lost · Moved:")
	if !strings.HasSuffix(row, "Not recorded · Recorded from 2026-06-15") {
		t.Errorf("an unrecorded week does not read as unrecorded: %q", row)
	}
}

// A source with no record at all has no date to give, which is every new seat.
func TestASourceWithNoRecordSaysSoWithoutADate(t *testing.T) {
	never := coverageOf(crmcontracts.WeeklyFigureCoverageStatusNotRecorded, nil)

	body := MailBody(coveredFixture(crmcontracts.WeeklyFigureCoverageSet{Deals: never}), "", english)

	if row := rowOf(t, body, "Won · Lost · Moved:"); !strings.HasSuffix(row, "Not recorded · No records from this source") {
		t.Errorf("a source with no record is not stated as such: %q", row)
	}
}

func TestAnUnknownZoneDatesInUTCAndSaysSo(t *testing.T) {
	review := coveredFixture(everyFamilyNotRecorded(firstRecord))
	review.NumericSummary.Timezone = "Not/AZone"

	if row := rowOf(t, MailBody(review, "", english), "Tasks delivered:"); !strings.HasSuffix(row, "Recorded from 2026-06-14 UTC") {
		t.Errorf("an unknown zone did not fall back to a labelled UTC date: %q", row)
	}
}

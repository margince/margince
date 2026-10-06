// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

import (
	"os"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// The health card's named dimensions (PO-AC-N-10..12, ADR-0095/A146).
//
// The rule under all of them: a dimension that cannot be READ is absent, never
// rated. Absence is a fact about the reading; a rating is a claim about the
// account, and the two must not render alike.

func ptrInt(v int) *int    { return &v }
func ptrBool(v bool) *bool { return &v }

// commercialStrip builds the state strip's commercial half — the only part
// these ratings read. Built by assigning fields rather than by a composite
// literal: the generated type is an anonymous struct, so restating its shape
// here would be a copy that drifts the moment the contract gains a field.
func commercialStrip(open, stalled int) *crmcontracts.Company360StateStrip {
	strip := &crmcontracts.Company360StateStrip{}
	strip.Commercial = new(struct {
		BaseCurrency          *string             `json:"base_currency,omitempty"`
		ConvertedCount        int                 `json:"converted_count"`
		FxAsOf                *openapi_types.Date `json:"fx_as_of,omitempty"`
		NextCloseOn           *openapi_types.Date `json:"next_close_on,omitempty"`
		OpenCount             int                 `json:"open_count"`
		OpenPipelineMinorBase *int                `json:"open_pipeline_minor_base,omitempty"`
		PricedCount           int                 `json:"priced_count"`
		StalledCount          int                 `json:"stalled_count"`
	})
	strip.Commercial.OpenCount = open
	strip.Commercial.StalledCount = stalled
	return strip
}

func TestRelationshipIsAbsentOnAnAccountNobodyHasReached(t *testing.T) {
	health := crmcontracts.Company360Health{ActiveContacts: ptrInt(0)}
	rateHealthDimensions(&health, nil, relstrength.ReadInTouch(nil, nil, nil, healthNow))

	// Not "at risk": an unstarted relationship is not a failing one, and rating
	// it would put a verdict on something that has not begun.
	if health.Relationship != nil {
		t.Fatalf("relationship = %+v, want absent on an account with no contact", health.Relationship)
	}
}

// healthNow is the instant every relationship case states its dates against.
var healthNow = time.Date(2026, 10, 6, 9, 0, 0, 0, time.UTC)

func healthDaysAgo(n int) *time.Time {
	at := healthNow.AddDate(0, 0, -n)
	return &at
}

// The relationship rating reads messages from them AND meetings with them,
// held or booked ahead, through the one in-touch rule relstrength holds.
func TestRelationshipReadsWhetherWeAreInTouch(t *testing.T) {
	cases := []struct {
		name           string
		activeContacts int
		lastInbound    *time.Time
		lastMeeting    *time.Time
		nextMeeting    *time.Time
		singleThreaded bool
		want           crmcontracts.HealthDimensionRating
		code           crmcontracts.HealthDimensionReasonCode
		reasonHas      string
	}{
		{
			name: "no message and no meeting, ever", activeContacts: 2,
			want: crmcontracts.HealthDimensionRatingAtRisk, code: crmcontracts.HealthDimensionReasonCodeNeverWritten,
			reasonHas: "never written",
		},
		{
			name: "old message and no meeting", activeContacts: 2, lastInbound: healthDaysAgo(77),
			want: crmcontracts.HealthDimensionRatingAtRisk, code: crmcontracts.HealthDimensionReasonCodeQuiet,
			reasonHas: "no meeting for 77 days",
		},
		{
			name: "old message and a meeting held too long ago", activeContacts: 2,
			lastInbound: healthDaysAgo(77), lastMeeting: healthDaysAgo(40),
			want: crmcontracts.HealthDimensionRatingAtRisk, code: crmcontracts.HealthDimensionReasonCodeQuiet,
			reasonHas: "no meeting for 40 days",
		},
		{
			name: "old message but a meeting three weeks ago", activeContacts: 2,
			lastInbound: healthDaysAgo(77), lastMeeting: healthDaysAgo(21),
			want: crmcontracts.HealthDimensionRatingStrong, code: crmcontracts.HealthDimensionReasonCodeLastMet,
			reasonHas: "Last met them 21 days ago",
		},
		{
			name: "old message but a meeting booked ahead", activeContacts: 2,
			lastInbound: healthDaysAgo(77), nextMeeting: healthDaysAgo(-2),
			want: crmcontracts.HealthDimensionRatingGood, code: crmcontracts.HealthDimensionReasonCodeMeetingBooked,
			reasonHas: "booked for 8 October 2026",
		},
		{
			name: "only meetings, never a message", activeContacts: 2, lastMeeting: healthDaysAgo(5),
			want: crmcontracts.HealthDimensionRatingStrong, code: crmcontracts.HealthDimensionReasonCodeLastMet,
			reasonHas: "Last met",
		},
		{
			name: "met recently, but one contact carries it", activeContacts: 1, lastMeeting: healthDaysAgo(21),
			singleThreaded: true,
			want:           crmcontracts.HealthDimensionRatingGood, code: crmcontracts.HealthDimensionReasonCodeLastMet,
			reasonHas: "Last met them 21 days ago",
		},
		{
			name: "in touch, but one contact carries it", activeContacts: 1, lastInbound: healthDaysAgo(3),
			singleThreaded: true,
			want:           crmcontracts.HealthDimensionRatingGood, code: crmcontracts.HealthDimensionReasonCodeSingleThreaded,
			reasonHas: "one contact",
		},
		{
			name: "several contacts, recently", activeContacts: 3, lastInbound: healthDaysAgo(3),
			want: crmcontracts.HealthDimensionRatingStrong, code: crmcontracts.HealthDimensionReasonCodeSeveralContacts,
			reasonHas: "3 contacts",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			health := crmcontracts.Company360Health{
				ActiveContacts: ptrInt(tc.activeContacts),
				SingleThreaded: ptrBool(tc.singleThreaded),
			}
			touch := relstrength.ReadInTouch(tc.lastInbound, tc.lastMeeting, tc.nextMeeting, healthNow)
			rateHealthDimensions(&health, nil, touch)

			got := health.Relationship
			if got == nil {
				t.Fatal("relationship is absent on an account with contacts")
			}
			if got.Rating != tc.want {
				t.Fatalf("rating = %q (%s), want %q", got.Rating, got.Reason, tc.want)
			}
			if got.ReasonCode == nil || *got.ReasonCode != tc.code {
				t.Fatalf("reason code = %v, want %q — without it the reader sees the English sentence in every language",
					got.ReasonCode, tc.code)
			}
			if !strings.Contains(got.Reason, tc.reasonHas) {
				t.Fatalf("reason = %q, want it to name %q — a rating with no sentence behind it is the unexplainable score this model replaced",
					got.Reason, tc.reasonHas)
			}
		})
	}
}

// The values a translated reason renders travel with the code, or the
// translation has nothing to say.
func TestRelationshipReasonCarriesTheValuesItNames(t *testing.T) {
	health := crmcontracts.Company360Health{ActiveContacts: ptrInt(2), SingleThreaded: ptrBool(false)}
	rateHealthDimensions(&health, nil, relstrength.ReadInTouch(healthDaysAgo(77), nil, healthDaysAgo(-2), healthNow))

	params := health.Relationship.ReasonParams
	if params == nil || params.On == nil || params.On.Format("2006-01-02") != "2026-10-08" {
		t.Fatalf("reason params = %+v, want the booked meeting's date 2026-10-08", params)
	}
}

func TestCommercialIsAbsentWhenTheReaderHasNoDealGrant(t *testing.T) {
	health := crmcontracts.Company360Health{ActiveContacts: ptrInt(2)}
	// A nil commercial half is the strip saying the READER cannot see deals.
	rateHealthDimensions(&health, &crmcontracts.Company360StateStrip{}, relstrength.InTouch{})

	if health.Commercial != nil {
		t.Fatalf("commercial = %+v, want absent — a withheld section is not a claim about the account", health.Commercial)
	}
}

// An account with no pipeline gets no commercial verdict.
//
// "No open deal" is not a risk: a customer under contract who is not being sold
// to today is in the ordinary state of a customer. Rated at risk it put a red
// verdict on an account that had done nothing to earn one, and the worst-of
// rule then carried that verdict into the account's overall standing.
func TestCommercialIsUnratedWhenNothingIsOpen(t *testing.T) {
	health := crmcontracts.Company360Health{}
	rateHealthDimensions(&health, commercialStrip(0, 0), relstrength.InTouch{})

	if health.Commercial != nil {
		t.Fatalf("commercial = %+v, want absent — there is no verdict to give on a pipeline that does not exist",
			health.Commercial)
	}
}

func TestCommercialReadsWhetherWorkIsMoving(t *testing.T) {
	cases := []struct {
		name      string
		open      int
		stalled   int
		want      crmcontracts.HealthDimensionRating
		reasonHas string
	}{
		{"everything stalled", 2, 2, crmcontracts.HealthDimensionRatingAtRisk, "All 2"},
		{"some stalled", 3, 1, crmcontracts.HealthDimensionRatingGood, "1 of 3"},
		{"all moving", 2, 0, crmcontracts.HealthDimensionRatingStrong, "none stalled"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			health := crmcontracts.Company360Health{}
			rateHealthDimensions(&health, commercialStrip(tc.open, tc.stalled), relstrength.InTouch{})

			if health.Commercial == nil {
				t.Fatal("commercial is absent where the strip carried a reading")
			}
			if health.Commercial.Rating != tc.want {
				t.Fatalf("rating = %q, want %q", health.Commercial.Rating, tc.want)
			}
			if !strings.Contains(health.Commercial.Reason, tc.reasonHas) {
				t.Fatalf("reason = %q, want it to name %q", health.Commercial.Reason, tc.reasonHas)
			}
		})
	}
}

// readHealth reads the state strip, so the strip must be assembled FIRST.
//
// The section list is an ordered literal inside `sections`, so this reads the
// source: reordering the two would leave commercial silently absent on every
// account, which renders as "nothing open" rather than as a bug. A comment
// could be ignored; this cannot.
func TestHealthIsAssembledAfterTheStateStripItReads(t *testing.T) {
	raw, err := os.ReadFile("assemble.go")
	if err != nil {
		t.Fatalf("read the assembler: %v", err)
	}
	source := string(raw)
	strip := strings.Index(source, "{sectionStateStrip, a.readStateStrip}")
	health := strings.Index(source, "{sectionHealth, a.readHealth}")
	if strip < 0 || health < 0 {
		t.Fatalf("state strip (%d) or health (%d) is no longer in the section list under that name", strip, health)
	}
	if strip > health {
		t.Fatal("health is assembled BEFORE the state strip it reads — commercial would be absent on every account, and absent renders as \"nothing open\"")
	}
}

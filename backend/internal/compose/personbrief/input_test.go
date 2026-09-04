// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package personbrief

// What the model is actually SHOWN, which is a different question from what the
// brief says.
//
// Every defect these cover is invisible to a test that asserts prose: a figure
// at the wrong scale, a held message whose words reached the prompt, or a
// timeline reduced to subjects and directions. Each of those looks like a model
// failure from the outside.

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The prompt carries a figure a reader can read, in the currency's own scale. A
// prompt carrying minor units once had a model read a 180,000 EUR deal as
// eighteen million and write that onto a screen whose own card said 180,000.
func TestTheDealAmountReachesTheModelAsAMajorUnitFigure(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		minor    int64
		currency string
		want     string
		reject   string
	}{
		{"a two-decimal currency", 18_000_000, "EUR", `"amount":"180000.00"`, "18000000"},
		// The other direction, and the one `/100` gets wrong: yen has no minor
		// unit, so the integer IS the figure and dividing would understate it a
		// hundredfold.
		{"a zero-decimal currency", 18_000_000, "JPY", `"amount":"18000000"`, "180000.00"},
		{"a three-decimal currency", 18_000_000, "KWD", `"amount":"18000.000"`, "180000.00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			encoded, err := json.Marshal(DealIn{
				ID: briefDealID, Name: "Acme renewal 2027", AmountMinor: tc.minor, Currency: tc.currency,
			})
			if err != nil {
				t.Fatalf("encoding the deal: %v", err)
			}
			got := string(encoded)
			if !strings.Contains(got, tc.want) {
				t.Errorf("the prompt carries %s, want %s — the model reads this figure as the "+
					"amount and writes it into prose a rep acts on", got, tc.want)
			}
			if strings.Contains(got, tc.reject) {
				t.Errorf("the prompt still carries %q in %s, which is the same money at the wrong scale",
					tc.reject, got)
			}
		})
	}
}

// The one field that decides whether this brief can say anything. A row reduced
// to kind, subject and direction supports exactly one sentence — "they emailed
// you about X" — which is the generic prose the model lane exists to replace.
func TestTheFoldCarriesWhatEachMessageActuallySaid(t *testing.T) {
	t.Parallel()
	in := FromView(viewFixture(t))
	if len(in.Recent) == 0 {
		t.Fatal("the fold carried no timeline at all")
	}
	if in.Recent[0].Preview != "Thursday at ten works for me." {
		t.Errorf("preview = %q, want the server's own line of what was written", in.Recent[0].Preview)
	}
	if in.Recent[0].Move != string(crmcontracts.EmailSummaryMoveNeedsReply) {
		t.Errorf("move = %q, want the server's reading of whose turn it is", in.Recent[0].Move)
	}
}

// A held message's words are not this reader's. The 360 nulls them; the fold
// must not reach past that, and must still carry the row so its DATE survives.
func TestAHeldMessageReachesTheModelAsADateAndNothingElse(t *testing.T) {
	t.Parallel()
	view := viewFixture(t)
	held := crmcontracts.ActivityContentStateWithheld
	row := view.Activities.Data[0]
	row.ContentState = &held
	row.Subject, row.Body = nil, nil
	row.EmailSummary = nil
	view.Activities.Data[0] = row

	in := FromView(view)
	if !in.Recent[0].Withheld {
		t.Error("the fold did not mark the held row, so the model cannot tell an empty message from a private one")
	}
	if in.Recent[0].Subject != "" || in.Recent[0].Preview != "" {
		t.Errorf("the held row reached the model as %+v, want its words absent", in.Recent[0])
	}
	if in.Recent[0].At == "" {
		t.Error("the held row lost its date, so a brief would report a silence that did not happen")
	}
}

// A dismissed claim is one a human said was never true. Writing a brief from it
// would resurrect it in prose.
func TestADismissedClaimNeverReachesTheModel(t *testing.T) {
	t.Parallel()
	view := viewFixture(t)
	claims := *view.Claims
	claims[0].Status = crmcontracts.ConversationClaimStatusDismissed
	view.Claims = &claims

	if in := FromView(view); len(in.Claims) != 0 {
		t.Errorf("the fold carried %+v, want a dismissed claim left out", in.Claims)
	}
}

// A relationship_change names no row, so citing it would send a reader to a
// record that does not exist. The activity and task rows behind a moment do.
func TestOnlyAMomentsRowBackedEvidenceBecomesACitation(t *testing.T) {
	t.Parallel()
	view := viewFixture(t)
	activity := openapi_types.UUID(ids.NewV7())
	derived := openapi_types.UUID(ids.NewV7())
	view.Moment = &crmcontracts.PersonMoment{
		Rule: crmcontracts.PersonMomentRuleOverduePromise, Headline: "You owe them the list",
		Evidence: []crmcontracts.PersonMomentEvidence{
			{Type: crmcontracts.PersonMomentEvidenceTypeActivity, Id: &activity},
			{Type: crmcontracts.PersonMomentEvidenceTypeRelationshipChange, Id: &derived},
			{Type: crmcontracts.PersonMomentEvidenceTypeActivity},
		},
	}
	in := FromView(view)
	if in.Moment == nil {
		t.Fatal("the fold dropped the moment")
	}
	want := []string{activity.String()}
	if len(in.Moment.Sources) != len(want) || in.Moment.Sources[0] != want[0] {
		t.Errorf("sources = %v, want only the row-backed evidence %v", in.Moment.Sources, want)
	}
}

// viewFixture is the assembled 360 the fold reads, with the one email row the
// cases above vary.
func viewFixture(t *testing.T) crmcontracts.Person360 {
	t.Helper()
	at := time.Date(2026, time.August, 29, 8, 10, 0, 0, time.UTC)
	subject := "Re: renewal call"
	preview := "Thursday at ten works for me."
	available := crmcontracts.ActivityContentStateAvailable
	direction := crmcontracts.ActivityDirectionInbound
	activityID := openapi_types.UUID(ids.NewV7())
	view := crmcontracts.Person360{
		Person: crmcontracts.Person{FullName: "Anna Weber"},
		Claims: &[]crmcontracts.ConversationClaim{{
			Id: openapi_types.UUID(ids.NewV7()), Kind: crmcontracts.Objection,
			Body:             "One listed sub-processor blocks legal sign-off.",
			Status:           crmcontracts.ConversationClaimStatusOpen,
			SourceQuote:      "we cannot go ahead while the analytics vendor is on it",
			SourceActivityId: activityID,
		}},
	}
	view.Activities = &struct {
		Data []crmcontracts.Activity `json:"data"`
		Page crmcontracts.PageInfo   `json:"page"`
	}{Data: []crmcontracts.Activity{{
		Id: activityID, Kind: crmcontracts.ActivityKindEmail, OccurredAt: at,
		Subject: &subject, Direction: &direction, ContentState: &available,
		EmailSummary: &crmcontracts.EmailSummary{
			ActivityId: activityID, OccurredAt: at, Subject: &subject, Preview: &preview,
			Move: crmcontracts.EmailSummaryMoveNeedsReply, DisplayStatus: crmcontracts.EmailAccessStatusTeam,
		},
	}}}
	return view
}

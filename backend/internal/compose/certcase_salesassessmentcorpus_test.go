// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/compose/salesassessment"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// askedInput keeps the input the case sent, so a reply is written against
// the ids that preparation minted.
type askedInput struct{ in salesassessment.Input }

func (a *askedInput) Complete(_ context.Context, req model.Request) (model.Response, error) {
	content := req.Messages[0].Content
	err := json.Unmarshal([]byte(content[strings.Index(content, "{"):strings.LastIndex(content, "}")+1]), &a.in)
	return model.Response{}, err
}

func TestTheUncoveredDealScenarioGradesTheReadingTheFigures(t *testing.T) {
	prepared := preparedCorpusCase(t, "aicert/corpus/report_interpret/sales_assessment_names_the_deal_no_decision_covers_01.yaml")
	var asked askedInput
	if _, err := prepared.Run(context.Background(), &asked); err != nil {
		t.Fatalf("running the case: %v", err)
	}
	deal := map[string]string{}
	for _, candidate := range asked.in.Candidates {
		deal[candidate.Name] = candidate.ID
	}
	linden, tern, decision := deal["Linden press line"], deal["Tern customer portal"], asked.in.Decisions[0].ID
	figures := `{"text":"You have won {{metric:bookings_won}} toward a landing of {{metric:forecast_landing}}.","kind":"observation","cells":["metric:bookings_won","metric:forecast_landing"]},` +
		`{"text":"The open pipeline is {{metric:open_pipeline}} and the closed win rate is {{metric:closed_win_rate}}.","kind":"observation","cells":["metric:open_pipeline","metric:closed_win_rate"]}`
	for name, tc := range map[string]struct{ reply, want string }{
		"a reading that names each deal and quotes its amount once": {
			reply: `{"paragraphs":[[` + figures + `,` +
				`{"text":"An open decision to agree the portal scope with the buyer's IT lead still covers the Tern customer portal.","kind":"claim","cells":["metric:open_pipeline"],"records":["` + decision + `","` + tern + `"]},` +
				`{"text":"The Linden press line at {{` + linden + `}} is the open deal no decision covers.","kind":"observation","cells":["metric:open_pipeline"],"records":["` + linden + `"]}` +
				`]],"next_decision":{"deal_id":"` + linden + `","text":"Decide who signs off the Linden press line this month."},"confidence":"medium"}`,
			want: aitasks.OutcomeAccepted,
		},
		"a reading that prints a deal's amount where its name belongs": {
			reply: `{"paragraphs":[[` + figures + `,` +
				`{"text":"You have an open decision regarding {{` + tern + `}} valued at {{` + tern + `}}.","kind":"claim","cells":["metric:open_pipeline"],"records":["` + decision + `","` + tern + `"]}` +
				`]],"next_decision":null,"confidence":"high"}`,
			want: aitasks.OutcomeInvalid,
		},
	} {
		if got := prepared.Evaluate(aitasks.Trace{Output: tc.reply}); got.Result != tc.want {
			t.Errorf("%s: %s (%s), want %s", name, got.Result, got.Detail, tc.want)
		}
	}
}

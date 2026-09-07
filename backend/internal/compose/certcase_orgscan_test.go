// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The account_scan/org_scan certification case. As with the brief's: a case
// no reply could fail measures nothing, so Prepare refuses such fixtures and
// expectations, and Evaluate tells the four outcomes apart.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const orgScanFixtureJSON = `{
  "name": "Nordlicht Logistik AG",
  "industry": "Logistics",
  "contacts": [{"label":"jonas","name":"Jonas Weber"}],
  "open_deals": [{"label":"retrofit","name":"Fleet retrofit","stage":"Proposal","amount_minor":4800000,"currency":"EUR"}],
  "open_tasks": [{"label":"call","name":"Call Jonas"}],
  "messages": [
    {"label":"ask","kind":"email","direction":"inbound","subject":"Telematics","at":"2026-08-19T08:45:00Z",
     "text":"Morning — did the sample reports get held up somewhere? The team meets on Thursday."},
    {"label":"thanks","kind":"email","direction":"inbound","subject":"Re: invoice","at":"2026-08-20T09:00:00Z",
     "text":"Thanks, the invoice arrived and is paid."}
  ]
}`

// preparedScan prepares the fixture, expecting the ask to be cited, and
// hands back the case itself, so a test can cite the ids it minted the way
// a correct model would.
func preparedScan(t *testing.T) *orgScanCase {
	t.Helper()
	prepared, err := orgScanCases{}.Prepare(json.RawMessage(orgScanFixtureJSON), json.RawMessage(`["ask"]`))
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	scan, ok := prepared.(*orgScanCase)
	if !ok {
		t.Fatalf("prepared case is %T, want the scan's own", prepared)
	}
	return scan
}

// scanReply is a model answer citing the labelled message with the given words.
func scanReply(t *testing.T, scan *orgScanCase, label, quote string) string {
	t.Helper()
	encoded, err := json.Marshal(map[string]any{"findings": []map[string]any{{
		"kind": "question_unanswered", "title": "Send the sample reports",
		"reason":     "Jonas asked for sample reports and nothing has gone out.",
		"message_id": scan.label[label], "quote": quote, "action": "draft_reply",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

type failingScanLane struct{ err error }

func (l failingScanLane) Complete(context.Context, model.Request) (model.Response, error) {
	return model.Response{}, l.err
}

func TestOrgScanCaseSiteIsTheAccountScansOneShot(t *testing.T) {
	site := orgScanCases{}.Site()
	if site.Variant != "org_scan" || string(site.Task) != "account_scan" {
		t.Errorf("Site() = %+v, want the account_scan/org_scan one-shot", site)
	}
}

func TestOrgScanCaseMintsTheIdsTheModelSeesAndSendsTheProductionRequest(t *testing.T) {
	first := preparedScan(t)
	second := preparedScan(t)
	if first.label["ask"] == second.label["ask"] {
		t.Error("two preparations of one fixture minted the same message id")
	}
	trace, err := first.Run(t.Context(), &recordingLane{reply: `{"findings":[]}`})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(trace.Requests) != 1 {
		t.Fatalf("the case issued %d requests, want one", len(trace.Requests))
	}
	sent := trace.Requests[0].Messages[len(trace.Requests[0].Messages)-1].Content
	if !strings.Contains(sent, first.label["ask"]) || strings.Contains(sent, `"ask"`) {
		t.Error("the prompt must carry the minted id and never the corpus label")
	}
	if trace.Output != `{"findings":[]}` {
		t.Errorf("trace output = %q, want the lane's reply", trace.Output)
	}
}

func TestOrgScanCaseCarriesTheLanesFailure(t *testing.T) {
	scan := preparedScan(t)
	refused := errors.New("lane closed")
	if _, err := scan.Run(t.Context(), failingScanLane{err: refused}); !errors.Is(err, refused) {
		t.Errorf("run: %v, want the lane's error carried", err)
	}
}

func TestOrgScanCaseRefusesWhatNoReplyCouldFail(t *testing.T) {
	for name, tc := range map[string]struct{ fixture, expected string }{
		"nothing expected":                  {orgScanFixtureJSON, `[]`},
		"a message the fixture lacks":       {orgScanFixtureJSON, `["a_message_that_is_not_there"]`},
		"a contact rather than an exchange": {orgScanFixtureJSON, `["jonas"]`},
		"an account with no exchange":       {`{"name":"Acme","messages":[]}`, `["ask"]`},
		"an unlabelled message":             {`{"name":"Acme","messages":[{"label":"","kind":"email","direction":"inbound","at":"2026-08-19T08:45:00Z","text":"hi"}]}`, `["x"]`},
		"a message dated in prose":          {`{"name":"Acme","messages":[{"label":"ask","kind":"email","direction":"inbound","at":"yesterday","text":"hi"}]}`, `["ask"]`},
		"two records with one label":        {`{"name":"Acme","contacts":[{"label":"ask","name":"J"}],"messages":[{"label":"ask","kind":"email","direction":"inbound","at":"2026-08-19T08:45:00Z","text":"hi"}]}`, `["ask"]`},
		"an unlabelled contact":             {`{"name":"Acme","contacts":[{"label":"","name":"J"}],"messages":[{"label":"ask","kind":"email","direction":"inbound","at":"2026-08-19T08:45:00Z","text":"hi"}]}`, `["ask"]`},
		"two deals with one label":          {`{"name":"Acme","open_deals":[{"label":"d","name":"A"},{"label":"d","name":"B"}],"messages":[{"label":"ask","kind":"email","direction":"inbound","at":"2026-08-19T08:45:00Z","text":"hi"}]}`, `["ask"]`},
		"an unlabelled task":                {`{"name":"Acme","open_tasks":[{"label":" ","name":"Call"}],"messages":[{"label":"ask","kind":"email","direction":"inbound","at":"2026-08-19T08:45:00Z","text":"hi"}]}`, `["ask"]`},
		"a fixture of the wrong shape":      {`["not","an","account"]`, `["ask"]`},
		"an expectation of the wrong shape": {orgScanFixtureJSON, `{"ask":true}`},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := (orgScanCases{}).Prepare(json.RawMessage(tc.fixture), json.RawMessage(tc.expected)); err == nil {
				t.Error("Prepare accepted a case no reply could disagree with")
			}
		})
	}
}

func TestOrgScanCaseEvaluatesEachOutcome(t *testing.T) {
	scan := preparedScan(t)
	for name, tc := range map[string]struct {
		output string
		want   string
	}{
		"cites the exchange the scenario names": {scanReply(t, scan, "ask", "did the sample reports get held up"), aitasks.OutcomeAccepted},
		"cites a different exchange":            {scanReply(t, scan, "thanks", "the invoice arrived and is paid"), aitasks.OutcomeWrongAnswer},
		"raises nothing":                        {`{"findings":[]}`, aitasks.OutcomeAbstained},
		"quotes words the message lacks":        {scanReply(t, scan, "ask", "we are cancelling"), aitasks.OutcomeInvalid},
		"did not answer at all":                 {"not json", aitasks.OutcomeInvalid},
	} {
		t.Run(name, func(t *testing.T) {
			got := scan.Evaluate(aitasks.Trace{Output: tc.output})
			if got.Result != tc.want {
				t.Errorf("Evaluate = %+v, want %s", got, tc.want)
			}
		})
	}
}

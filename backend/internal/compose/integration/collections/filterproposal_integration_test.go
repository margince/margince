// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package collections

// Proposing a filter from plain words, over the composed server with the model
// stubbed at its one true boundary: the lane's Complete.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// recordingLane answers one canned reply and keeps every request it was sent,
// so a test can read what reached the model.
type recordingLane struct {
	mu    sync.Mutex
	reply string
	sent  []model.Request
}

func (l *recordingLane) Complete(_ context.Context, req model.Request) (model.Response, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sent = append(l.sent, req)
	return model.Response{Text: l.reply, ServedModel: "stub-model"}, nil
}

func (l *recordingLane) prompts() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, 0, len(l.sent))
	for _, req := range l.sent {
		for _, message := range req.Messages {
			out = append(out, message.Content)
		}
	}
	return out
}

func (l *recordingLane) firstSystem() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.sent) == 0 {
		return ""
	}
	return l.sent[0].System
}

type proposalBody struct {
	Resource    string          `json:"resource"`
	Filter      json.RawMessage `json:"filter"`
	Unsupported []struct {
		Phrase string `json:"phrase"`
		Code   string `json:"code"`
		Field  string `json:"field"`
	} `json:"unsupported"`
	ModelUsed string `json:"model_used"`
	Code      string `json:"code"`
}

func proposalEnv(t *testing.T, lane *recordingLane) *apptest.AppEnv {
	t.Helper()
	opts := []compose.Option{compose.WithSchemaPool(integration.SchemaPool(t))}
	if lane != nil {
		opts = append(opts, compose.WithFilterProposals(lane))
	}
	e := apptest.SetupAppWithOptions(t, opts...)
	e.BootstrapWorkspace(t)
	return e
}

func cannedProposal(column string) string {
	return `{"groups":[{"join":"and","clauses":[
	  {"phrase":"gold tier","field":"` + column + `","op":"eq","text":"gold","number":null,"flag":null,"list":null,"days_ago":null},
	  {"phrase":"with high intent","field":"purchase_intent","op":"eq","text":"high","number":null,"flag":null,"list":null,"days_ago":null}]}],
	 "join":"and","unsupported":[{"phrase":"likely to buy","reason":"No field records a prediction."}]}`
}

// The proposal is checked before it is answered, lands in the shape the preview
// evaluates, reads no record, and writes nothing.
func TestAPlainWordsProposalIsCheckedAndPreviewable(t *testing.T) {
	lane := &recordingLane{}
	e := proposalEnv(t, lane)
	column := seedContactsWithTier(t, e, 3, 2)
	lane.reply = cannedProposal(column)
	before := ledgers(t, e)

	var got proposalBody
	if status := e.Call(t, "POST", "/v1/filters/propose", integration.AnyMap{
		"resource": "contact", "text": "gold tier contacts with high intent, likely to buy", "locale": "de",
	}, nil, &got); status != http.StatusOK {
		t.Fatalf("propose: status=%d body=%+v", status, got)
	}
	codes := map[string]string{}
	for _, item := range got.Unsupported {
		codes[item.Phrase] = item.Code
	}
	if codes["with high intent"] != "unknown_field" || codes["likely to buy"] != "not_expressible" || len(codes) != 2 {
		t.Errorf("unsupported = %+v, want the unknown field dropped and the model's own phrase kept", got.Unsupported)
	}
	if got.ModelUsed != "stub-model" {
		t.Errorf("model_used = %q, want the model that answered", got.ModelUsed)
	}

	var preview previewBody
	if status := e.Call(t, "POST", "/v1/filters/preview", integration.AnyMap{
		"resource": "contact", "filter": got.Filter,
	}, nil, &preview); status != http.StatusOK {
		t.Fatalf("the proposed filter does not preview: status=%d filter=%s", status, got.Filter)
	}
	if preview.MatchCount != 3 {
		t.Errorf("the proposed filter selects %d, want the 3 gold contacts", preview.MatchCount)
	}

	prompt := strings.Join(lane.prompts(), "\n")
	if !strings.Contains(prompt, "Preview Tier") || !strings.Contains(prompt, column) {
		t.Error("the prompt does not carry the custom field and its label, so 'tier' names nothing")
	}
	if strings.Contains(prompt, "Preview Subject") {
		t.Error("a contact's name reached the model; it is handed the vocabulary, never records")
	}
	if !strings.Contains(lane.firstSystem(), "German") {
		t.Error("the reasons are not asked for in the reader's language")
	}
	if after := ledgers(t, e); after != before {
		t.Errorf("a proposal wrote to the ledgers: before %+v, after %+v", before, after)
	}
}

// A deployment with no model answers a problem the builder can show, never a 500.
func TestAPlainWordsProposalWithNoModelSaysOneIsNeeded(t *testing.T) {
	e := proposalEnv(t, nil)
	var got proposalBody
	status := e.Call(t, "POST", "/v1/filters/propose", integration.AnyMap{
		"resource": "company", "text": "companies in Germany",
	}, nil, &got)
	if status != http.StatusConflict || got.Code != "ai_not_configured" {
		t.Errorf("status=%d code=%q, want 409 ai_not_configured", status, got.Code)
	}
}

// A caller who may not read the record type is refused before any model call:
// the budget is not spent to tell somebody they could not have used the answer.
func TestAPlainWordsProposalNeedsReadOnTheRecordType(t *testing.T) {
	lane := &recordingLane{reply: `{"groups":[],"join":"and","unsupported":[]}`}
	e := proposalEnv(t, lane)
	if _, err := e.Owner.Exec(context.Background(), `
		UPDATE role SET permissions = jsonb_set(
			permissions, '{objects,company}',
			'{"create": false, "read": false, "update": false, "delete": false}'::jsonb, true)
		 WHERE is_system`); err != nil {
		t.Fatalf("revoking company.read: %v", err)
	}
	var got proposalBody
	if status := e.Call(t, "POST", "/v1/filters/propose", integration.AnyMap{
		"resource": "company", "text": "companies in Germany",
	}, nil, &got); status != http.StatusForbidden {
		t.Errorf("status=%d body=%+v, want 403", status, got)
	}
	if len(lane.prompts()) != 0 {
		t.Error("the model was asked on behalf of a caller who may not read companies")
	}
}

// The sentence is bounded the way the contract declares, and refused by name.
func TestAPlainWordsProposalRefusesAnEmptyOrOversizedSentence(t *testing.T) {
	lane := &recordingLane{reply: `{"groups":[],"join":"and","unsupported":[]}`}
	e := proposalEnv(t, lane)
	for name, text := range map[string]string{"blank": "   ", "too long": strings.Repeat("a", 501)} {
		t.Run(name, func(t *testing.T) {
			if status := e.Call(t, "POST", "/v1/filters/propose", integration.AnyMap{
				"resource": "contact", "text": text,
			}, nil, nil); status != http.StatusUnprocessableEntity {
				t.Errorf("status=%d, want 422", status)
			}
		})
	}
	if len(lane.prompts()) != 0 {
		t.Error("a refused sentence still reached the model")
	}
}

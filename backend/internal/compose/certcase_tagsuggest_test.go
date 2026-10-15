// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The tag suggestion case issues the request the pass issues and judges the
// reply with the pass's validator. A broken batch is invalid, never wrong.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/aitasks"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// tagSuggestCompleterStub answers from the request it is sent: the item ids
// it fences, and the tag ids it lists, both minted by Prepare.
type tagSuggestCompleterStub struct {
	answer func(items, tags []string) string
	seen   []model.Request
}

func (s *tagSuggestCompleterStub) Complete(_ context.Context, req model.Request) (model.Response, error) {
	s.seen = append(s.seen, req)
	items, err := requestedIDsIn(req)
	if err != nil {
		return model.Response{}, err
	}
	return model.Response{Text: s.answer(items, listedTagIDs(req.Messages[0].Content))}, nil
}

// listedTagIDs reads the tag ids off the vocabulary lines of a prompt, in order.
func listedTagIDs(prompt string) []string {
	var out []string
	for line := range strings.Lines(prompt) {
		id, _, found := strings.Cut(strings.TrimPrefix(line, "- "), ":")
		if _, err := ids.Parse(id); found && strings.HasPrefix(line, "- ") && err == nil {
			out = append(out, id)
		}
	}
	return out
}

func tagSuggestFixtureJSON(t *testing.T) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(tagSuggestFixture{
		Tags: []tagSuggestFixtureTag{
			{Key: "migration", Name: "Ledger migration", Description: "wants to move their bookkeeping off spreadsheets"},
			{Key: "support", Name: "Premium support", Description: "wants faster help"},
		},
		Items: []tagSuggestFixtureItem{
			{Kind: "email", Subject: "Month-end", Body: "We still close the month in Excel."},
			{Kind: "note", Subject: "Lunch", Body: "Thanks for lunch."},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func runTagSuggestCase(t *testing.T, expected string, answer func(items, tags []string) string) (aitasks.Outcome, *tagSuggestCompleterStub) {
	t.Helper()
	prepared, err := tagSuggestCases{}.Prepare(tagSuggestFixtureJSON(t), json.RawMessage(expected))
	if err != nil {
		t.Fatalf("preparing: %v", err)
	}
	stub := &tagSuggestCompleterStub{answer: answer}
	trace, err := prepared.Run(context.Background(), stub)
	if err != nil {
		t.Fatalf("running: %v", err)
	}
	return prepared.Evaluate(trace), stub
}

func TestTagSuggestCaseSeparatesARightAWrongAndABrokenReply(t *testing.T) {
	const expected = `[["migration"], []]`
	cases := map[string]struct {
		answer func(items, tags []string) string
		want   string
	}{
		"right": {func(items, tags []string) string {
			return `{"items":[{"id":"` + items[0] + `","tags":["` + tags[0] + `"]},{"id":"` + items[1] + `","tags":[]}]}`
		}, aitasks.OutcomeAccepted},
		"a mention tagged": {func(items, tags []string) string {
			return `{"items":[{"id":"` + items[0] + `","tags":["` + tags[0] + `"]},{"id":"` + items[1] + `","tags":["` + tags[1] + `"]}]}`
		}, aitasks.OutcomeWrongAnswer},
		"an unlisted tag": {func(items, _ []string) string {
			return `{"items":[{"id":"` + items[0] + `","tags":["` + ids.NewV7().String() + `"]},{"id":"` + items[1] + `","tags":[]}]}`
		}, aitasks.OutcomeInvalid},
		"an item left out": {func(items, tags []string) string {
			return `{"items":[{"id":"` + items[0] + `","tags":["` + tags[0] + `"]}]}`
		}, aitasks.OutcomeInvalid},
		"an unsent item": {func(items, tags []string) string {
			return `{"items":[{"id":"` + items[0] + `","tags":["` + tags[0] + `"]},{"id":"` + ids.NewV7().String() + `","tags":[]}]}`
		}, aitasks.OutcomeInvalid},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got, _ := runTagSuggestCase(t, expected, c.answer); got.Result != c.want {
				t.Errorf("outcome = %q (%s), want %q", got.Result, got.Detail, c.want)
			}
		})
	}
}

// The case issues the request the pass builds. Items are fenced, and the
// schema holds item ids and tag ids to the ones sent.
func TestTagSuggestCaseIssuesTheFencedRequestOnce(t *testing.T) {
	_, stub := runTagSuggestCase(t, `[[], []]`, func(items, _ []string) string {
		return `{"items":[{"id":"` + items[0] + `","tags":[]},{"id":"` + items[1] + `","tags":[]}]}`
	})
	if len(stub.seen) != 1 {
		t.Fatalf("the case made %d calls, want the one call it certifies", len(stub.seen))
	}
	req := stub.seen[0]
	items, err := requestedIDsIn(req)
	if err != nil || len(items) != 2 {
		t.Fatalf("fenced items = %v (err %v), want 2", items, err)
	}
	for _, id := range append(items, listedTagIDs(req.Messages[0].Content)...) {
		if !strings.Contains(string(req.ResponseSchema), id) {
			t.Errorf("the response schema does not hold %s to the ids this call sent", id)
		}
	}
	if strings.Contains(string(req.ResponseSchema), "confidence") {
		t.Error("the schema asks for a confidence number the pass never reads")
	}
}

func TestTagSuggestCaseRefusesAnExpectationNoReplyCanMeet(t *testing.T) {
	for name, expected := range map[string]string{
		"one answer for two items": `[["migration"]]`,
		"a tag the fixture lacks":  `[["pricing"], []]`,
	} {
		if _, err := (tagSuggestCases{}).Prepare(tagSuggestFixtureJSON(t), json.RawMessage(expected)); err == nil {
			t.Errorf("%s: Prepare accepted %s", name, expected)
		}
	}
}

func TestTagSuggestCaseRefusesABatchThePassCouldNeverSend(t *testing.T) {
	long := strings.Repeat("x", tagSuggestBodyLimit+1)
	for name, fixture := range map[string]tagSuggestFixture{
		"no tag":       {Items: []tagSuggestFixtureItem{{Kind: "note", Body: "x"}}},
		"a long body":  {Tags: []tagSuggestFixtureTag{{Key: "k"}}, Items: []tagSuggestFixtureItem{{Kind: "note", Body: long}}},
		"a key reused": {Tags: []tagSuggestFixtureTag{{Key: "k"}, {Key: "k"}}, Items: []tagSuggestFixtureItem{{Kind: "note"}}},
	} {
		raw, err := json.Marshal(fixture)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := (tagSuggestCases{}).Prepare(raw, json.RawMessage(`[[]]`)); err == nil {
			t.Errorf("%s: Prepare accepted a batch the pass never sends", name)
		}
	}
}

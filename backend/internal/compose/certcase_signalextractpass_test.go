// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// queuedCompleter answers each request with the next reply, in order.
type queuedCompleter struct {
	replies  []func(c *signalExtractCase) string
	requests []model.Request
	c        *signalExtractCase
}

func (q *queuedCompleter) Complete(_ context.Context, req model.Request) (model.Response, error) {
	reply := q.replies[len(q.requests)](q.c)
	q.requests = append(q.requests, req)
	return model.Response{Text: reply}, nil
}

const promiseThenNeedFixture = `[
	{"direction":"outbound","body":"I will send you the edited recording once it is ready."},
	{"direction":"inbound","body":"We would like to join the beta.","reported":"%s"},
	{"direction":"outbound","body":"Jana Weber runs the beta."}]`

func prepareSignalExtract(t *testing.T, fixture string) *signalExtractCase {
	t.Helper()
	prepared, err := signalExtractCases{}.Prepare(json.RawMessage(fixture), json.RawMessage(`[]`))
	if err != nil {
		t.Fatal(err)
	}
	c, ok := prepared.(*signalExtractCase)
	if !ok {
		t.Fatalf("Prepare returned %T, want *signalExtractCase", prepared)
	}
	return c
}

func needOn(message int) func(c *signalExtractCase) string {
	return func(c *signalExtractCase) string {
		return `{"events":[{"kind":"new_opportunity","message_id":"` + c.numbered[message-1].ID.String() +
			`","summary":"They want the beta.","confidence":0.9}]}`
	}
}

func promiseOn(message int) func(c *signalExtractCase) string {
	return func(c *signalExtractCase) string {
		return `{"events":[{"kind":"commitment_made","message_id":"` + c.numbered[message-1].ID.String() +
			`","summary":"Send the recording.","confidence":0.9}]}`
	}
}

func fixedExtractReply(text string) func(*signalExtractCase) string {
	return func(*signalExtractCase) string { return text }
}

func TestACertifiedReadingRunsTheWholePassTheProductRuns(t *testing.T) {
	unreported := strings.Replace(promiseThenNeedFixture, `,"reported":"%s"`, "", 1)
	cases := map[string]struct {
		fixture  string
		replies  []func(*signalExtractCase) string
		calls    int
		served   []int
		withNeed bool
	}{
		"a promise the first reading left out is read again beside the recorded need": {
			unreported, []func(*signalExtractCase) string{needOn(2), promiseOn(1)}, 2, []int{2, 1}, false,
		},
		"a refused second reply leaves the first reading": {
			unreported, []func(*signalExtractCase) string{needOn(2), fixedExtractReply("not json")}, 2, []int{2}, false,
		},
		"a fixture that is already the second reading makes one call": {
			strings.Replace(promiseThenNeedFixture, "%s", extractKindOpportunity, 1),
			[]func(*signalExtractCase) string{promiseOn(1)}, 1, []int{1}, false,
		},
		"an unusable first reply makes no second call": {
			unreported, []func(*signalExtractCase) string{fixedExtractReply(`{"answer":"none"}`)}, 1, nil, true,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := prepareSignalExtract(t, tc.fixture)
			brain := &queuedCompleter{replies: tc.replies, c: c}

			trace, err := c.Run(context.Background(), brain)

			if err != nil {
				t.Fatal(err)
			}
			if len(brain.requests) != tc.calls || len(trace.Requests) != tc.calls {
				t.Fatalf("the pass made %d calls and traced %d, want %d", len(brain.requests), len(trace.Requests), tc.calls)
			}
			if last := brain.requests[len(brain.requests)-1].Messages[0].Content; tc.calls == 2 &&
				!strings.Contains(last, "Already recorded, so report none of it again: new_opportunity in m2") {
				t.Error("the second reading is not told that the new need is recorded")
			}
			var served extractPayload
			if err := json.Unmarshal([]byte(trace.Output), &served); err != nil && !tc.withNeed {
				t.Fatalf("the graded output is not a reading: %v", err)
			}
			position := map[string]int{}
			for i, m := range c.numbered {
				position[m.ID.String()] = i + 1
			}
			var got []int
			for _, event := range served.events() {
				got = append(got, position[event.MessageID])
			}
			if !slices.Equal(got, tc.served) {
				t.Errorf("the graded answer cites messages %v, want %v", got, tc.served)
			}
		})
	}
}

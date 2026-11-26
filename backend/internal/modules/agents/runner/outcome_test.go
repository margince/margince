// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package runner

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func TestNotWrittenOnceNamesEachListedSubjectOffOneWrite(t *testing.T) {
	cases := map[string]struct {
		listed, written []string
		want            []Tally
	}{
		"each once":          {[]string{"a", "b"}, []string{"b", "a"}, nil},
		"one left unwritten": {[]string{"a", "b"}, []string{"a"}, []Tally{{Subject: "b"}}},
		"one written twice":  {[]string{"a", "b"}, []string{"a", "a", "b"}, []Tally{{Subject: "a", Writes: 2}}},
		"listed twice":       {[]string{"a", "a"}, nil, []Tally{{Subject: "a"}}},
		"off-list write":     {[]string{"a"}, []string{"a", "z"}, nil},
		"nothing listed":     {nil, []string{"z"}, nil},
	}
	for name, tc := range cases {
		if got := NotWrittenOnce(tc.listed, tc.written); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %+v, want %+v", name, got, tc.want)
		}
	}
}

// The check reads each executed call's whole answer, never the trace's cut copy.
func TestAnOutcomeCheckReadsWholeAnswersAndCanSendTheFinalBack(t *testing.T) {
	long := `{"tail":"` + strings.Repeat("x", 3*traceObservationLimit) + `"}`
	surface := &fakeSurface{results: map[string]json.RawMessage{"read_record": json.RawMessage(long)}}
	brain := &scriptedBrain{texts: []string{
		`{"tool":"read_record","args":{"id":"x"}}`,
		`{"final":{"summary":"done"}}`,
		`{"final":{"summary":"done"}}`,
	}}
	var seen [][]Call
	check := func(calls []Call) (string, error) {
		seen = append(seen, calls)
		if len(seen) == 1 {
			return "record x needs a note", nil
		}
		return "", nil
	}
	res, err := New(surface, brain).Run(context.Background(),
		Job{Goal: "g", Tools: []string{"read_record"}, Outcome: check})
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != OutcomeCompleted || len(seen) != 2 {
		t.Fatalf("outcome = %q after %d checks, want completed after 2", res.Outcome, len(seen))
	}
	if got := string(seen[0][0].Answer); got != long {
		t.Errorf("the check read %d bytes of a %d-byte answer", len(got), len(long))
	}
	refusal := lastMessage(brain.requests[2])
	if !strings.Contains(refusal, "record x needs a note") || !strings.Contains(refusal, outcomeRefusedDirective) {
		t.Errorf("the refusal must carry the check's finding and the directive: %q", refusal)
	}
}

// A suspended run stores its checked calls with the snapshot, so the resumed
// final answer is judged on the run's whole history.
func TestAResumedRunIsCheckedOnTheCallsMadeBeforeItSuspended(t *testing.T) {
	stored, err := json.Marshal(Pending{
		ApprovalID: ids.New[ids.ApprovalKind](), Tool: "update_record", Args: json.RawMessage(`{}`),
		Window: []model.Message{{Role: "user", Content: "Goal: g"}}, Fence: promptfence.New(),
		TranscriptVersion: neutralisedObservations,
		Calls:             []Call{{Tool: "read_record", Answer: json.RawMessage(`{"before":true}`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var pending Pending
	if err := json.Unmarshal(stored, &pending); err != nil {
		t.Fatal(err)
	}
	var seen []Call
	check := func(calls []Call) (string, error) { seen = calls; return "", nil }
	brain := &scriptedBrain{texts: []string{`{"final":{"summary":"done"}}`}}
	_, err = New(&fakeSurface{}, brain).Resume(context.Background(),
		Job{Goal: "g", Tools: []string{"update_record"}, Outcome: check}, Decision{Pending: pending})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 1 || seen[0].Tool != "read_record" {
		t.Fatalf("the check saw %+v, want the call made before the run suspended", seen)
	}
}

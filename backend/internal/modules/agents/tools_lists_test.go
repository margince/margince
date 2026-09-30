// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

// stubLists answers every list call with an empty result, and remembers the
// last call it was handed.
type stubLists struct {
	read   *ListRead
	change *ListChange
}

func (s *stubLists) ReadLists(_ context.Context, q ListRead) (json.RawMessage, error) {
	s.read = &q
	return json.RawMessage(`{}`), nil
}

func (s *stubLists) ChangeLists(_ context.Context, c ListChange) (json.RawMessage, error) {
	s.change = &c
	return json.RawMessage(`{}`), nil
}

func TestAListCallMissingWhatItsModeNeedsNeverReachesTheSeam(t *testing.T) {
	for name, tc := range map[string]struct {
		tool, args string
	}{
		"get without a list":         {"read_lists", `{"mode":"get"}`},
		"why without a record":       {"read_lists", `{"mode":"why","list_id":"019ff000-0000-7000-8000-000000000001"}`},
		"preview without a filter":   {"read_lists", `{"mode":"preview","entity_type":"contact"}`},
		"update without a version":   {"change_lists", `{"mode":"update","list_id":"019ff000-0000-7000-8000-000000000001"}`},
		"add without a record type":  {"change_lists", `{"mode":"add_member","list_id":"019ff000-0000-7000-8000-000000000001","record_id":"019ff000-0000-7000-8000-000000000002"}`},
		"create without a name":      {"change_lists", `{"mode":"create","entity_type":"contact"}`},
		"a mode neither tool speaks": {"change_lists", `{"mode":"merge"}`},
	} {
		t.Run(name, func(t *testing.T) {
			seam := &stubLists{}
			r := NewRegistry(nil, nil)
			RegisterListTools(r, seam)
			tool, ok := r.tools[tc.tool]
			if !ok {
				t.Fatalf("%s is not registered", tc.tool)
			}
			_, err := tool.Handle(context.Background(), json.RawMessage(tc.args))
			var bad *BadArgsError
			if !errors.As(err, &bad) {
				t.Fatalf("answered %v, want a BadArgsError", err)
			}
			if seam.read != nil || seam.change != nil {
				t.Fatalf("the seam was reached with an incomplete call")
			}
		})
	}
}

func TestAListCallAnswersInTheModeItWasAskedIn(t *testing.T) {
	seam := &stubLists{}
	r := NewRegistry(nil, nil)
	RegisterListTools(r, seam)
	tool := r.tools["change_lists"]
	out, err := tool.Handle(context.Background(), json.RawMessage(
		`{"mode":"add_member","list_id":"019ff000-0000-7000-8000-000000000001","entity_type":"company","record_id":"019ff000-0000-7000-8000-000000000002","note":"reference customer"}`))
	if err != nil {
		t.Fatal(err)
	}
	var answer ListsAnswer
	if err := json.Unmarshal(out, &answer); err != nil {
		t.Fatal(err)
	}
	if answer.Mode != ListModeAdd || seam.change == nil || *seam.change.Note != "reference customer" {
		t.Fatalf("answer %+v, seam saw %+v", answer, seam.change)
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAUserSumsTheirPassportsAndIsAsStoppedAsTheirWorst(t *testing.T) {
	holder := AgentUsageHolder{UserID: ids.NewV7(), DisplayName: "Minh", Subjects: []string{"a", "b"}}
	usage := map[string]agentvolume.Usage{
		"a": {Writes: agentvolume.CounterUsage{Used: 200, Limit: 200}},
		"b": {Writes: agentvolume.CounterUsage{Used: 0, Limit: 200}},
	}

	row := userUsage(holder, usage)

	if row.Writes.Used != 200 || row.Writes.Limit != 400 || row.PassportCount != 2 {
		t.Errorf("the row sums to %+v over %d passports, want 200 of 400 over 2", row.Writes, row.PassportCount)
	}
	if row.State != "stopped" {
		t.Errorf("one stopped passport left the user %q, want stopped: the sum hides it", row.State)
	}
}

// The row answers how much and whose, never which agents.
func TestAgentUsageNamesNoPassport(t *testing.T) {
	row := userUsage(AgentUsageHolder{UserID: ids.NewV7(), DisplayName: "Minh", Subjects: []string{"a"}},
		map[string]agentvolume.Usage{"a": {}})
	raw, err := json.Marshal(row)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	want := []string{"calls", "display_name", "egress", "passport_count", "state", "user_id", "writes"}
	if !slices.Equal(keys, want) {
		t.Errorf("a user's row carries %v, want exactly %v", keys, want)
	}
}

func TestAConnectionIsOneSubjectHoweverManyPassportsItHas(t *testing.T) {
	user := ids.NewV7()
	holders := addHolderSubject(nil, user, "Minh", "grant")
	holders = addHolderSubject(holders, user, "Minh", "grant")
	holders = addHolderSubject(holders, user, "Minh", "minted")
	holders = addHolderSubject(holders, ids.NewV7(), "Anna", "other")

	if len(holders) != 2 || !slices.Equal(holders[0].Subjects, []string{"grant", "minted"}) {
		t.Errorf("holders are %+v, want Minh with grant and minted, then Anna", holders)
	}
}

// The reset comes from the window the counts were read in. A second look at
// the meter could land in the next window.
func TestTheResetIsTheOneTheCountsWereReadIn(t *testing.T) {
	counted := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	reader := &recordedUsage{}
	h := Handlers{}.WithPassportUsage(reader)

	got := h.resetsAt(context.Background(), map[string]agentvolume.Usage{"a": {ResetsAt: counted}})

	if !got.Equal(counted) {
		t.Errorf("the reset is %v, want %v from the counted window", got, counted)
	}
}

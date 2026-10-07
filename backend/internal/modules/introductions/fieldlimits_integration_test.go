// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package introductions

import (
	"context"
	"strings"
	"testing"
)

func (e *introEnv) storedCount(t *testing.T) int {
	t.Helper()
	var n int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM intro_request`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// The contract counts characters, so a Vietnamese reason is held to the same
// number as an English one, and text past it is refused rather than cut.
func TestAnAskCountsCharactersAndRefusesWhatIsTooLong(t *testing.T) {
	e := setupIntro(t)
	cases := []struct {
		name   string
		field  string
		mutate func(*NewRequest)
	}{
		{"reason over", "internal_reason", func(r *NewRequest) { r.InternalReason = strings.Repeat("a", 2001) }},
		{"value over", "value_for_target", func(r *NewRequest) { r.ValueForTarget = strings.Repeat("é", 2001) }},
		{"note over", "forwardable_note", func(r *NewRequest) { r.ForwardableNote = strings.Repeat("ế", 4001) }},
		{"blank reason", "internal_reason", func(r *NewRequest) { r.InternalReason = "" }},
		{"unknown fallback", "fallback_policy", func(r *NewRequest) { r.FallbackPolicy = "zzz" }},
		{"unknown note origin", "note_generated_by", func(r *NewRequest) { r.NoteGeneratedBy = "zzz" }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := e.ask()
			c.mutate(&req)
			_, err := e.store.Create(e.asUser(e.requester), req)
			if got := refusedField(t, err); got != c.field {
				t.Errorf("refused on %q, want %q", got, c.field)
			}
		})
	}
	if n := e.storedCount(t); n != 0 {
		t.Fatalf("%d asks stored after only refusals", n)
	}

	req := e.ask()
	req.InternalReason = strings.Repeat("ế", 2000)
	id, err := e.store.Create(e.asUser(e.requester), req)
	if err != nil {
		t.Fatalf("2000 multibyte characters are within the contract: %v", err)
	}
	var got string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT internal_reason FROM intro_request WHERE id = $1`, id).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != req.InternalReason {
		t.Errorf("stored %d characters, want the %d sent", len([]rune(got)), 2000)
	}
}

func TestAnAnswerAndAWithdrawalRefuseAReasonOverTheLimit(t *testing.T) {
	e := setupIntro(t)
	id, err := e.store.Create(e.asUser(e.requester), e.ask())
	if err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("é", 2001)

	err = e.store.Decide(e.asUser(e.introducer), id, StatusDeclined, long, nil, 1)
	if got := refusedField(t, err); got != "reason" {
		t.Errorf("decision refused on %q, want reason", got)
	}
	err = e.store.Cancel(e.asUser(e.requester), id, long, 1)
	if got := refusedField(t, err); got != "reason" {
		t.Errorf("cancel refused on %q, want reason", got)
	}
	ok := strings.Repeat("é", 2000)
	if err := e.store.Cancel(e.asUser(e.requester), id, ok, 1); err != nil {
		t.Fatalf("2000 characters withdraw cleanly: %v", err)
	}
}

// An ask that states neither enum is a contact typing with no fallback, and
// the store records exactly that.
func TestAnAskThatStatesNoEnumIsStoredAsHumanWithNoFallback(t *testing.T) {
	e := setupIntro(t)
	req := e.ask()
	req.NoteGeneratedBy, req.FallbackPolicy = "", ""
	id, err := e.store.Create(e.asUser(e.requester), req)
	if err != nil {
		t.Fatal(err)
	}
	var origin, fallback string
	if err := e.owner.QueryRow(context.Background(),
		`SELECT note_generated_by, fallback_policy FROM intro_request WHERE id = $1`, id).Scan(&origin, &fallback); err != nil {
		t.Fatal(err)
	}
	if origin != "human" || fallback != "none" {
		t.Errorf("stored %q and %q, want human and none", origin, fallback)
	}
}

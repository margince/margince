// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"encoding/json"
	"testing"
)

// TestPersonRelinkFor covers every state an activity already on file can be in
// when the person reconciliation reaches it.
//
// Each relink is a write on a surface that stamps a six-year retention class
// the database will not let anyone lift, and this one also DELETES the link it
// replaces — so "do nothing" is the answer that has to be right most often.
func TestPersonRelinkFor(t *testing.T) {
	const signer, senior = "p-signer", "p-senior"
	mail := demoActivity{Company: "acme.test", Kind: "email", Person: "Karoline Juettner"}

	for _, tc := range []struct {
		name     string
		act      demoActivity
		existing seededActivity
		want     string
		wantID   string
		wantMove bool
	}{
		{
			// The defect this pass exists for. The installation was seeded
			// before activities named a counterpart per entry, so the link
			// points at whoever sorted most senior — and every re-seed replays
			// the create instead of repairing it.
			name:     "an activity on the wrong person is moved to the signer",
			act:      mail,
			existing: seededActivity{ID: "a1", PersonIDs: []string{senior}},
			want:     signer,
			wantID:   signer,
			wantMove: true,
		}, {
			// Seeded before activities carried a person link at all.
			name:     "an activity linked to nobody is filed against the signer",
			act:      mail,
			existing: seededActivity{ID: "a1"},
			want:     signer,
			wantID:   signer,
			wantMove: true,
		}, {
			// A converged installation. The pass must be silent, or every run
			// rewrites links nothing changed.
			name:     "an activity already on the right person is left alone",
			act:      mail,
			existing: seededActivity{ID: "a1", PersonIDs: []string{signer}},
			want:     signer,
			wantMove: false,
		}, {
			// MORE than one, which this tool never writes: a capture that
			// resolved participants, or somebody associating a colleague by
			// hand. The move would DELETE those. Leaving a wrong link standing
			// is recoverable; deleting a fact this tool did not create is not.
			name:     "an activity carrying links this tool did not write is left alone",
			act:      mail,
			existing: seededActivity{ID: "a1", PersonIDs: []string{senior, "p-colleague"}},
			want:     signer,
			wantMove: false,
		}, {
			// Even when one of them is already right — the count is the
			// question, not whether the wanted id happens to be among them.
			name:     "an activity carrying the right person AND another is left alone",
			act:      mail,
			existing: seededActivity{ID: "a1", PersonIDs: []string{signer, "p-colleague"}},
			want:     signer,
			wantMove: false,
		}, {
			// A note is about the account, not with anybody. Filing one
			// against a person would invent a conversation that never happened.
			name:     "a note is left alone",
			act:      demoActivity{Company: "acme.test", Kind: "note"},
			existing: seededActivity{ID: "a1"},
			want:     signer,
			wantMove: false,
		}, {
			name:     "a task is left alone",
			act:      demoActivity{Company: "acme.test", Kind: "task"},
			existing: seededActivity{ID: "a1"},
			want:     signer,
			wantMove: false,
		}, {
			// The ~180 crawled companies with nobody on file. The create path
			// links no person either, so there is nothing to repair towards.
			name:     "an account with no staff at all is left alone",
			act:      mail,
			existing: seededActivity{ID: "a1"},
			want:     "",
			wantMove: false,
		}, {
			// And the same when a link is already there: with no counterpart
			// to move to, deleting the one on file would leave the timeline
			// emptier than it found it.
			name:     "an account with no staff does not lose the link it has",
			act:      mail,
			existing: seededActivity{ID: "a1", PersonIDs: []string{senior}},
			want:     "",
			wantMove: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			gotID, gotMove := personRelinkFor(tc.act, tc.existing, tc.want)
			if gotMove != tc.wantMove {
				t.Fatalf("move = %v, want %v", gotMove, tc.wantMove)
			}
			if gotMove && gotID != tc.wantID {
				t.Errorf("person = %q, want %q", gotID, tc.wantID)
			}
		})
	}
}

// TestTheIndexReadsEveryPersonLink — the count is what the repair turns on, so
// a page carrying two must not arrive as one.
//
// Keeping only the last would make an activity carrying somebody else's
// participant look like an ordinary single-linked row, and the repair would
// then delete that participant on the next re-seed.
func TestTheIndexReadsEveryPersonLink(t *testing.T) {
	page := json.RawMessage(`[
	  {"id":"a1","source_system":"seed","source_id":"act-3","occurred_at":"2026-01-22T09:00:00Z",
	   "links":[{"entity_type":"organization","entity_id":"org-1"},
	            {"entity_type":"person","entity_id":"p-1"},
	            {"entity_type":"person","entity_id":"p-2"}]}
	]`)
	seen := map[string]seededActivity{}
	if err := indexSeededActivities(page, seen); err != nil {
		t.Fatalf("indexing the page: %v", err)
	}
	got := seen["act-3"].PersonIDs
	if len(got) != 2 {
		t.Fatalf("read %v, want both person links", got)
	}
	if got[0] != "p-1" || got[1] != "p-2" {
		t.Errorf("read %v, want [p-1 p-2]", got)
	}
}

// TestTheCounterpartRulingBelongsToOneFunction — a note or a task is with
// nobody, and counterpartFor says so before it reads anything.
//
// The ruling lives there rather than at the call site because the create path
// and the repair both have to make it the same way. Driven with a nil client
// on purpose: if the kind check ever moves below the first read, this panics
// rather than passing.
func TestTheCounterpartRulingBelongsToOneFunction(t *testing.T) {
	for _, kind := range []string{"note", "task"} {
		got, err := counterpartFor(nil, demoActivity{Company: "acme.test", Kind: kind}, "org-1")
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		if got != "" {
			t.Errorf("%s named counterpart %q — an internal record is with nobody", kind, got)
		}
	}
}

// TestTheCreateWritesTheCounterpartItWasHanded — the create writes the
// counterpart it is handed, which is what lets the repair read that answer
// instead of working one out for itself.
//
// activityLinks used to resolve the counterpart itself. While it did, the
// repair pass had to ask the same question a second time; a second answer that
// differed would relink activities nothing had changed, on a surface whose
// retention stamp the database will not let anyone lift.
func TestTheCreateWritesTheCounterpartItWasHanded(t *testing.T) {
	refs := refsWithProjects()
	act := demoActivity{Company: "acme.test", Kind: "email", DaysAgo: 30}

	links := activityLinks(act.Kind, "p-signer", "org-acme", refs, act)
	if got := linkedEntity(links, "person"); got != "p-signer" {
		t.Errorf("the create linked person %q, want the counterpart it was handed", got)
	}
	// And no counterpart means no person link, rather than a link to nothing.
	links = activityLinks(act.Kind, "", "org-acme", refs, act)
	if got := linkedEntity(links, "person"); got != "" {
		t.Errorf("the create linked person %q with no counterpart resolved", got)
	}
	// The company is linked either way: that is what makes the row appear on
	// the account at all.
	if got := linkedEntity(links, "organization"); got != "org-acme" {
		t.Errorf("the create linked organization %q, want org-acme", got)
	}
}

func linkedEntity(links []jsonBody, entityType string) string {
	for _, link := range links {
		if link["entity_type"] == entityType {
			id, _ := link["entity_id"].(string)
			return id
		}
	}
	return ""
}

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

// TestActivityIdentityMatches — a source id here is the entry's POSITION, so
// the question "is this the row this entry is about" has to be asked twice.
//
// The organization catches a shift across companies. It cannot catch one
// WITHIN a company, where the dataset holds several entries against the same
// account and reordering them renames each other's rows — and that is the case
// that files a mail against the wrong colleague, which is the exact defect the
// person repair exists to fix.
func TestActivityIdentityMatches(t *testing.T) {
	const org = "org-acme"
	act := demoActivity{Company: "acme.test", Kind: "email", Subject: "Rueckfrage zur Migration"}

	for _, tc := range []struct {
		name     string
		subject  string
		existing seededActivity
		orgID    string
		want     bool
	}{
		{
			name:     "the row this entry wrote",
			existing: seededActivity{OrganizationID: org, Subject: act.Subject},
			orgID:    org,
			want:     true,
		}, {
			name:     "a row on another company",
			existing: seededActivity{OrganizationID: "org-other", Subject: act.Subject},
			orgID:    org,
			want:     false,
		}, {
			// The shift the organization check is blind to: two entries on ONE
			// account swapped places, so act-3 now names the other one's mail.
			name:     "another entry's row on the same company",
			existing: seededActivity{OrganizationID: org, Subject: "Angebot Zweiter Mandant"},
			orgID:    org,
			want:     false,
		}, {
			// A call or a meeting often carries no subject. The empty subject
			// is a FACT about the entry, not a wildcard: matching any stored
			// row is how a subjectless call reaches another entry's mail,
			// because the entry names nothing for anything to disagree with.
			name:     "an entry with no subject does not match a row that has one",
			subject:  "-",
			existing: seededActivity{OrganizationID: org, Subject: "whatever the server holds"},
			orgID:    org,
			want:     false,
		}, {
			name:     "an entry with no subject matches the row that has none",
			subject:  "-",
			existing: seededActivity{OrganizationID: org},
			orgID:    org,
			want:     true,
		}, {
			// The post-seed verification runs with no domain map, so it passes
			// no organization. The subject still answers.
			name:     "with no company resolved the subject still answers",
			existing: seededActivity{OrganizationID: "org-other", Subject: act.Subject},
			want:     true,
		}, {
			name:     "with no company resolved a different subject is refused",
			existing: seededActivity{OrganizationID: org, Subject: "Angebot Zweiter Mandant"},
			want:     false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := act
			if tc.subject == "-" {
				// The dataset names none, which is itself the fingerprint.
				entry.Subject = ""
			}
			if got := activityIdentityMatches(entry, tc.existing, tc.orgID); got != tc.want {
				t.Errorf("matches = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestTheIndexReadsWhatARepairNeedsToPinItself — the version and the subject
// are read off the page or neither guard exists at all.
//
// The version goes back as If-Match, so a row somebody moved between the
// snapshot and the repair is refused rather than overwritten; the subject is
// half the row's identity. Both are silent if the index drops them: a zero
// version would be sent as a pin no row can satisfy, and an empty subject
// would make every entry's identity check pass.
func TestTheIndexReadsWhatARepairNeedsToPinItself(t *testing.T) {
	page := json.RawMessage(`[
	  {"id":"a1","source_system":"seed","source_id":"act-9","occurred_at":"2026-01-22T09:00:00Z",
	   "subject":"Rueckfrage zur Migration","version":7,
	   "links":[{"entity_type":"organization","entity_id":"org-1"}]}
	]`)
	seen := map[string]seededActivity{}
	if err := indexSeededActivities(page, seen); err != nil {
		t.Fatalf("indexing the page: %v", err)
	}
	got := seen["act-9"]
	if got.Version != 7 {
		t.Errorf("version = %d, want 7 — the repair would pin a version no row carries", got.Version)
	}
	if got.Subject != "Rueckfrage zur Migration" {
		t.Errorf("subject = %q, want the stored one — every identity check would pass without it", got.Subject)
	}
}

// TestAmbiguousEntriesAreLeftAlone — two entries the dataset cannot tell apart.
//
// Identity is (company, subject), and where that pair is not unique the only
// thing left is the POSITION, which is not durable: source ids are positional
// and inserting an entry renames every row after it. Two subjectless calls on
// one account are the common case — neither entry names anything the other
// does not — and a repair that guessed would replace one mail's counterpart
// with the other's.
func TestAmbiguousEntriesAreLeftAlone(t *testing.T) {
	cfg := demoConfig{Activities: []demoActivity{
		{Company: "acme.test", Kind: "call"},
		{Company: "acme.test", Kind: "call"},
		{Company: "acme.test", Kind: "email", Subject: "Rueckfrage zur Migration"},
		{Company: "other.test", Kind: "call"},
		// The same subject twice on one account is the same trap spelled out.
		{Company: "other.test", Kind: "email", Subject: "Angebot"},
		{Company: "other.test", Kind: "email", Subject: "Angebot"},
	}}
	ambiguous := ambiguousEntries(cfg, true)
	for _, i := range []int{0, 1, 4, 5} {
		if !ambiguous[i] {
			t.Errorf("entry %d shares its (company, subject) with another and was not named as ambiguous", i)
		}
	}
	for _, i := range []int{2, 3} {
		if ambiguous[i] {
			t.Errorf("entry %d is the only one of its (company, subject) and was named as ambiguous — the repair would stop reaching it", i)
		}
	}
	// The post-seed check cannot resolve a stored row's account, so for it the
	// SUBJECT alone has to be unique. Entry 2 is the only one of its subject
	// and stays checkable; the two "Angebot" mails are a pair either way; and
	// the subjectless calls are shared across companies, which the passes can
	// separate and this cannot.
	subjectOnly := ambiguousEntries(cfg, false)
	if !subjectOnly[3] {
		t.Error("a subjectless call on another company is separable only by the account, which the post-seed check cannot resolve — it must be skipped there")
	}
	if subjectOnly[2] {
		t.Error("the only entry carrying its subject was skipped by the weaker identity too — the check would stop reaching anything")
	}

	// A company named with different casing is the SAME company, or the
	// ambiguity check would miss a pair the org resolution treats as one.
	mixed := demoConfig{Activities: []demoActivity{
		{Company: "Acme.Test", Kind: "call"},
		{Company: "acme.test", Kind: "call"},
	}}
	if got := ambiguousEntries(mixed, true); !got[0] || !got[1] {
		t.Errorf("two subjectless calls on one account spelled with different casing read as unambiguous: %v", got)
	}
}

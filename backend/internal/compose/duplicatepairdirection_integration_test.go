// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The direction of a one-sided duplicate merge through the contact case and
// for leads. Also the queue's authority flags on every route that answers a pair.

import (
	"context"
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestACasePairMergeKeepsTheColleaguesRecordAndItsOwner(t *testing.T) {
	c := setupCases(t)
	cased := c.seedCase(t, c.e.Rep1, c.e.Rep2)
	mine, theirs := cased[0], cased[1]
	rep := c.rep(c.e.Rep1)
	read := c.read(rep, t, mine)

	for _, refused := range []struct {
		survivor, owner ids.UUID
		code            string
	}{
		{mine, c.e.Rep1, "merge_direction"},
		{mine, c.e.Rep2, "merge_direction"},
		{theirs, c.e.Rep1, "owner_not_eligible"},
	} {
		if _, err := c.store.MergeContactCase(rep, mine, caseDecision(read, cased, refused.survivor, refused.owner)); caseCode(err) != refused.code {
			t.Errorf("keeping %s for owner %s = %v, want %s", refused.survivor, refused.owner, err, refused.code)
		}
	}
	out, err := c.store.MergeContactCase(rep, mine, caseDecision(read, cased, theirs, c.e.Rep2))
	if err != nil {
		t.Fatalf("giving one's own contact away in the case: %v", err)
	}
	if out.Mode != crmcontracts.ContactCaseOutcomeModePair || out.Survivor == nil ||
		out.Survivor.OwnerId == nil || ids.UUID(*out.Survivor.OwnerId) != c.e.Rep2 {
		t.Errorf("outcome = %+v, want a pair merge keeping the colleague as owner", out)
	}
}

type leadPairs struct {
	c   *caseEnv
	rep context.Context
}

func newLeadPairs(c *caseEnv) leadPairs {
	perms := caseRepPerms
	perms.Objects = map[string]principal.ObjectGrant{"lead": {Create: true, Read: true, Update: true}}
	rep := c.e.As(c.e.Rep1, nil, perms)
	actor, _ := principal.Actor(rep)
	actor.SeatType = principal.SeatFull
	return leadPairs{c: c, rep: principal.WithActor(rep, actor)}
}

func (l leadPairs) lead(ctx context.Context, t *testing.T, name, email string, owner ids.UUID) ids.LeadID {
	t.Helper()
	user := ids.From[ids.UserKind](owner)
	created, _, err := l.c.store.CreateLead(ctx, contacts.CreateLeadInput{FullName: &name, Email: &email, OwnerID: &user, Source: "manual"})
	if err != nil {
		t.Fatalf("creating lead %s: %v", name, err)
	}
	return ids.From[ids.LeadKind](ids.UUID(created.Id))
}

func (l leadPairs) filed(t *testing.T, a, b ids.LeadID) int {
	t.Helper()
	return l.c.e.WsCount(t, `SELECT count(*) FROM dedupe_candidate WHERE entity_type = 'lead'
		AND left_lead_id = LEAST($1::uuid, $2::uuid) AND right_lead_id = GREATEST($1::uuid, $2::uuid)`, a, b)
}

// collide files a pair the way a rep could make one: their own lead, renamed
// after a colleague's.
func (l leadPairs) collide(t *testing.T, name string) (mine, theirs ids.LeadID) {
	t.Helper()
	theirs = l.lead(l.c.e.Admin(), t, name, "their-"+ids.NewV7().String()+"@lead.test", l.c.e.Rep2)
	mine = l.lead(l.rep, t, "Quentin Apart", "mine-"+ids.NewV7().String()+"@lead.test", l.c.e.Rep1)
	if n := l.filed(t, mine, theirs); n != 0 {
		t.Fatalf("the unalike leads were filed %d time(s)", n)
	}
	if _, err := l.c.store.MergeLead(l.rep, mine, theirs); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("a merge into a colleague's unfiled lead = %v, want the bare conflict", err)
	}
	if _, err := l.c.store.UpdateLead(l.rep, mine, contacts.UpdateLeadInput{FullName: &name}); err != nil {
		t.Fatalf("renaming the rep's own lead: %v", err)
	}
	if n := l.filed(t, mine, theirs); n != 1 {
		t.Fatalf("the rename filed the pair %d time(s), want once", n)
	}
	return mine, theirs
}

func (l leadPairs) ownerOf(t *testing.T, lead ids.LeadID) string {
	t.Helper()
	return l.c.e.WsScalar(t, `SELECT owner_id::text FROM lead WHERE id = $1`, lead)
}

func TestTheOwnerOfOneLeadOfAFiledPairOnlyGivesTheirsAway(t *testing.T) {
	c := setupCases(t)
	l := newLeadPairs(c)

	mine, theirs := l.collide(t, "Otto Collide")
	if _, err := c.store.MergeLead(l.rep, theirs, mine); !isMergeDirection(err) {
		t.Errorf("archiving the colleague's lead = %v, want the merge_direction refusal", err)
	}
	merged, err := c.store.MergeLead(l.rep, mine, theirs)
	if err != nil {
		t.Fatalf("giving one's own lead away: %v", err)
	}
	if merged.OwnerId == nil || ids.UUID(*merged.OwnerId) != c.e.Rep2 {
		t.Error("the surviving lead changed owner")
	}

	mine, theirs = l.collide(t, "Petra Queue")
	pair := leadPairID(t, c, mine, theirs)
	keepMine, keepTheirs := mine.UUID, theirs.UUID
	if _, err := c.store.DisposeDedupeCandidate(l.rep, pair, "merge", &keepMine); !isMergeDirection(err) {
		t.Errorf("a queue merge keeping the rep's own lead = %v, want the merge_direction refusal", err)
	}
	if _, err := c.store.DisposeDedupeCandidate(l.rep, pair, "merge", &keepTheirs); err != nil {
		t.Fatalf("a queue merge giving the rep's own lead away: %v", err)
	}
	if got := l.ownerOf(t, theirs); got != c.e.Rep2.String() {
		t.Errorf("the surviving lead is owned by %s, want the colleague", got)
	}

	mine, theirs = l.collide(t, "Rosa Dismissed")
	if _, err := c.store.DisposeDedupeCandidate(c.e.Admin(), leadPairID(t, c, mine, theirs), "not_a_duplicate", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.store.MergeLead(l.rep, mine, theirs); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("a merge into a colleague's lead of a dismissed pair = %v, want the bare conflict", err)
	}
}

func leadPairID(t *testing.T, c *caseEnv, a, b ids.LeadID) ids.UUID {
	t.Helper()
	id, err := ids.Parse(c.e.WsScalar(t, `SELECT id::text FROM dedupe_candidate WHERE entity_type = 'lead'
		AND $1 IN (left_lead_id, right_lead_id) AND $2 IN (left_lead_id, right_lead_id)`, a, b))
	if err != nil {
		t.Fatalf("reading the filed lead pair: %v", err)
	}
	return id
}

// pairFlags is what a reader is told they may decide about one pair.
type pairFlags struct {
	decide, merge bool
	keep          string
}

func flagsOf(row contacts.DedupeCandidateRow) pairFlags {
	out := pairFlags{decide: row.CanDecide, merge: row.CanMerge}
	if row.KeepID != nil {
		out.keep = row.KeepID.String()
	}
	return out
}

func reportedFlags(candidate agents.DuplicateCandidate) pairFlags {
	out := pairFlags{merge: candidate.CanMerge}
	if candidate.KeepID != nil {
		out.keep = *candidate.KeepID
	}
	return out
}

func (c *caseEnv) listedFlags(ctx context.Context, t *testing.T, status string, id ids.UUID) pairFlags {
	t.Helper()
	rows, _, err := c.store.ListDedupeCandidates(ctx, contacts.DedupeQueueInput{Status: status, EntityType: "contact", Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ID == id {
			return flagsOf(row)
		}
	}
	t.Fatalf("the %s lane does not list the pair", status)
	return pairFlags{}
}

func TestEveryRouteThatAnswersAPairTellsTheSameAuthority(t *testing.T) {
	c := setupCases(t)
	half := c.seedCase(t, c.e.Rep1, c.e.Rep2)
	halfPair := c.pairID(t, half[0], half[1])
	rep := c.rep(c.e.Rep1)
	got, err := c.store.GetDedupeCandidate(rep, halfPair)
	if err != nil {
		t.Fatal(err)
	}
	listed := c.listedFlags(rep, t, "open", halfPair)
	if flagsOf(got) != listed || listed != (pairFlags{merge: true, keep: half[1].String()}) {
		t.Errorf("the read answers %+v and the list %+v, want both to offer the merge keeping the colleague's record", flagsOf(got), listed)
	}
	reported, err := openDuplicatesFor(c.e.Pool)(rep, "contact", half[0], nil)
	if err != nil || len(reported) != 1 {
		t.Fatalf("the create's report = %+v, %v, want the one filed pair", reported, err)
	}
	if got := reportedFlags(reported[0]); got != (pairFlags{merge: listed.merge, keep: listed.keep}) {
		t.Errorf("the create reports %+v and the list %+v, want the same merge offer", got, listed)
	}

	both := c.seedCase(t, c.e.Rep1, c.e.Rep1)
	id := c.pairID(t, both[0], both[1])
	open := pairFlags{decide: true, merge: true}
	if got := c.listedFlags(rep, t, "open", id); got != open {
		t.Errorf("the owner of both lists %+v, want %+v", got, open)
	}
	steps := []struct {
		name, status string
		run          func() (contacts.DedupeCandidateRow, error)
	}{
		{"dismiss", "not_a_duplicate", func() (contacts.DedupeCandidateRow, error) {
			return c.store.DisposeDedupeCandidate(rep, id, "not_a_duplicate", nil)
		}},
		{"undo", "open", func() (contacts.DedupeCandidateRow, error) { return c.store.UndoDedupeDisposition(rep, id) }},
		{"merge", "merged", func() (contacts.DedupeCandidateRow, error) {
			return c.store.DisposeDedupeCandidate(rep, id, "merge", &both[0])
		}},
	}
	for _, step := range steps {
		answered, err := step.run()
		if err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
		if listed := c.listedFlags(rep, t, step.status, id); flagsOf(answered) != listed {
			t.Errorf("%s answers %+v and the %s lane lists %+v", step.name, flagsOf(answered), step.status, listed)
		}
		if step.status == "open" && flagsOf(answered) != open {
			t.Errorf("%s answers %+v, want %+v", step.name, flagsOf(answered), open)
		}
	}
}

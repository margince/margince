// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Who may merge a filed duplicate pair, over the real merge with its consent,
// introduction and grant carriers wired, acted on by own-scope reps. The owner
// of either half merges it; a merge of anything else still needs write
// authority over both records.

import (
	"context"
	"errors"
	"maps"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func contactID(id ids.UUID) ids.ContactID { return ids.From[ids.ContactKind](id) }

// pairID reads the queue row the creates filed between two contacts.
func (c *caseEnv) pairID(t *testing.T, a, b ids.UUID) ids.UUID {
	t.Helper()
	raw := c.e.WsScalar(t, `SELECT id::text FROM dedupe_candidate WHERE entity_type = 'contact'
		AND left_contact_id = LEAST($1::uuid, $2::uuid) AND right_contact_id = GREATEST($1::uuid, $2::uuid)`, a, b)
	id, err := ids.Parse(raw)
	if err != nil {
		t.Fatalf("reading the filed pair: %v", err)
	}
	return id
}

// mergedBy counts the merge audit rows one rep left on the two records.
func (c *caseEnv) mergedBy(t *testing.T, rep ids.UUID, records ...ids.UUID) int {
	t.Helper()
	return c.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE action = 'merge' AND entity_type = 'contact'
		AND entity_id = ANY($1) AND actor_id = $2`, records, "human:"+rep.String())
}

func TestTheOwnerOfEitherHalfMergesAFiledPair(t *testing.T) {
	c := setupCases(t)
	rep := c.rep(c.e.Rep1)
	for _, keepMine := range []bool{true, false} {
		pair := c.seedCase(t, c.e.Rep1, c.e.Rep2)
		mine, theirs := pair[0], pair[1]
		source, target, owner := theirs, mine, c.e.Rep1
		if !keepMine {
			source, target, owner = mine, theirs, c.e.Rep2
		}
		if _, err := c.store.PreviewMergeSurvivorship(c.previewer(c.e.Rep1), "contact", source, target, nil, nil); err != nil {
			t.Fatalf("keep mine %v: the preview refused the owner of one half: %v", keepMine, err)
		}
		survivor, err := c.store.MergeContact(rep, contactID(source), contactID(target), nil)
		if err != nil {
			t.Fatalf("keep mine %v: the owner of one half was refused: %v", keepMine, err)
		}
		if survivor.OwnerId == nil || ids.UUID(*survivor.OwnerId) != owner {
			t.Errorf("keep mine %v: the survivor's owner changed, want the target's owner %s", keepMine, owner)
		}
		if n := c.mergedBy(t, c.e.Rep1, source, target); n != 2 {
			t.Errorf("keep mine %v: %d merge audit rows name the rep on the two records, want 2", keepMine, n)
		}
		if got := c.e.WsScalar(t, `SELECT disposition FROM dedupe_candidate WHERE id = $1`, c.pairID(t, mine, theirs)); got != "merged" {
			t.Errorf("keep mine %v: the filed pair reads %s, want merged", keepMine, got)
		}
	}
}

func TestTheOwnerOfEitherHalfMergesThroughTheQueueAndTheCase(t *testing.T) {
	c := setupCases(t)
	queued := c.seedCase(t, c.e.Rep1, c.e.Rep2)
	listed, _, err := c.store.ListDedupeCandidates(c.rep(c.e.Rep2), contacts.DedupeQueueInput{
		Status: "open", EntityType: "contact", Limit: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || !listed[0].CanMerge || listed[0].CanDecide {
		t.Fatalf("the owner of one half lists %+v, want the pair with can_merge and without can_decide", listed)
	}
	winner := queued[0]
	if _, err := c.store.DisposeDedupeCandidate(c.rep(c.e.Rep2), c.pairID(t, queued[0], queued[1]), "merge", &winner); err != nil {
		t.Fatalf("the owner of the loser merging through the queue: %v", err)
	}

	cased := c.seedCase(t, c.e.Rep1, c.e.Rep2)
	read := c.read(c.rep(c.e.Rep2), t, cased[0])
	if read.MergeMode != crmcontracts.ContactCaseMergeModePair || !read.CanMerge {
		t.Fatalf("the owner of one half reads merge_mode %s and can_merge %v, want pair and true", read.MergeMode, read.CanMerge)
	}
	out, err := c.store.MergeContactCase(c.rep(c.e.Rep2), cased[0], caseDecision(read, cased, cased[0], c.e.Rep1))
	if err != nil {
		t.Fatalf("the owner of one half merging the case: %v", err)
	}
	if !out.Committed || out.Mode != crmcontracts.ContactCaseOutcomeModePair {
		t.Errorf("outcome = %+v, want a committed pair merge", out)
	}
}

func TestAMergeOutsideAFiledPairStillNeedsBothRecords(t *testing.T) {
	c := setupCases(t)
	rep := c.rep(c.e.Rep1)

	neither := c.seedCase(t, c.e.Rep2, c.e.Rep3)
	if _, err := c.store.MergeContact(rep, contactID(neither[0]), contactID(neither[1]), nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a merge of two colleagues' filed pair = %v, want 403", err)
	}
	winner := neither[0]
	if _, err := c.store.DisposeDedupeCandidate(rep, c.pairID(t, neither[0], neither[1]), "merge", &winner); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a queue merge of two colleagues' filed pair = %v, want 403", err)
	}

	mine := c.unlike(t, "Ingrid Unfiled", c.e.Rep1)
	theirs := c.unlike(t, "Mateo Elsewhere", c.e.Rep2)
	if n := c.e.WsCount(t, `SELECT count(*) FROM dedupe_candidate WHERE $1 IN (left_contact_id, right_contact_id)`, mine); n != 0 {
		t.Fatalf("the unalike contacts were filed as a pair %d time(s)", n)
	}
	if _, err := c.store.MergeContact(rep, contactID(mine), contactID(theirs), nil); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("a merge of one's own record into a colleague's unfiled one = %v, want the bare conflict", err)
	}
	if _, err := c.store.MergeContact(rep, contactID(theirs), contactID(mine), nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a merge of a colleague's unfiled record into one's own = %v, want 403", err)
	}

	dismissed := c.seedCase(t, c.e.Rep1, c.e.Rep2)
	if _, err := c.store.DisposeDedupeCandidate(c.e.Admin(), c.pairID(t, dismissed[0], dismissed[1]), "not_a_duplicate", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := c.store.MergeContact(rep, contactID(dismissed[1]), contactID(dismissed[0]), nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a merge of a pair someone marked as two people = %v, want 403", err)
	}
}

// unlike creates a contact no other contact in the test resembles.
func (c *caseEnv) unlike(t *testing.T, name string, owner ids.UUID) ids.UUID {
	t.Helper()
	user := ids.From[ids.UserKind](owner)
	created, err := c.store.CreateContact(c.e.Admin(), contacts.CreateContactInput{FullName: name, Source: "manual", OwnerID: &user})
	if err != nil {
		t.Fatalf("creating %s: %v", name, err)
	}
	return ids.UUID(created.Id)
}

// previewer is an own-scope rep holding every object grant, since a merge
// preview reads what the merge carries across modules.
func (c *caseEnv) previewer(user ids.UUID) context.Context {
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["introduction"] = principal.ObjectGrant{Read: true}
	perms.RoleKeys = []string{"rep"}
	perms.RowScope = principal.RowScopeOwn
	ctx := c.e.As(user, nil, perms)
	actor, _ := principal.Actor(ctx)
	actor.SeatType = principal.SeatFull
	return principal.WithActor(ctx, actor)
}

func TestTheOwnerOfEitherHalfMergesAFiledCompanyPair(t *testing.T) {
	c := setupCases(t)
	perms := caseRepPerms
	perms.Objects = map[string]principal.ObjectGrant{"company": {Create: true, Read: true, Update: true}}
	rep := c.e.As(c.e.Rep1, nil, perms)
	actor, _ := principal.Actor(rep)
	actor.SeatType = principal.SeatFull
	rep = principal.WithActor(rep, actor)

	company := func(name, domain string, owner ids.UUID) ids.UUID {
		user := ids.From[ids.UserKind](owner)
		created, err := c.store.CreateCompany(c.e.Admin(), contacts.CreateCompanyInput{
			DisplayName: name, Source: "manual", OwnerID: &user,
			Domains: []contacts.CompanyDomainInput{{Domain: domain, IsPrimary: true}},
		})
		if err != nil {
			t.Fatalf("creating %s: %v", name, err)
		}
		return ids.UUID(created.Id)
	}
	mine := company("Globex Corporation GmbH", "globex.test", c.e.Rep1)
	theirs := company("Globex Corporation Inc", "globex-us.test", c.e.Rep2)
	if n := c.e.WsCount(t, `SELECT count(*) FROM dedupe_candidate WHERE entity_type = 'company' AND disposition = 'open'
		AND $1 IN (left_company_id, right_company_id) AND $2 IN (left_company_id, right_company_id)`, mine, theirs); n != 1 {
		t.Fatalf("the two companies were filed %d time(s), want once", n)
	}
	if _, err := c.store.MergeCompany(rep, ids.From[ids.CompanyKind](theirs), ids.From[ids.CompanyKind](mine), nil); err != nil {
		t.Fatalf("the owner of one company of a filed pair was refused: %v", err)
	}
	stranger := c.unlikeCompany(t, c.e.Rep2)
	if _, err := c.store.MergeCompany(rep, ids.From[ids.CompanyKind](stranger), ids.From[ids.CompanyKind](mine), nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a merge of a colleague's unfiled company = %v, want 403", err)
	}
}

func (c *caseEnv) unlikeCompany(t *testing.T, owner ids.UUID) ids.UUID {
	t.Helper()
	user := ids.From[ids.UserKind](owner)
	created, err := c.store.CreateCompany(c.e.Admin(), contacts.CreateCompanyInput{
		DisplayName: "Initech Holdings", Source: "manual", OwnerID: &user,
		Domains: []contacts.CompanyDomainInput{{Domain: "initech.test", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("creating the unalike company: %v", err)
	}
	return ids.UUID(created.Id)
}

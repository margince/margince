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

// Each owner of a filed pair may give their own record away into the other's.
// Neither may archive the colleague's record into their own.
func TestEachOwnerOfAFiledPairMergesTheirRecordIntoTheColleagues(t *testing.T) {
	c := setupCases(t)
	for _, merger := range []ids.UUID{c.e.Rep1, c.e.Rep2} {
		pair := c.seedCase(t, c.e.Rep1, c.e.Rep2)
		mine, theirs, colleague := pair[0], pair[1], c.e.Rep2
		if merger == c.e.Rep2 {
			mine, theirs, colleague = pair[1], pair[0], c.e.Rep1
		}
		rep := c.rep(merger)
		if _, err := c.store.PreviewMergeSurvivorship(c.previewer(merger), "contact", theirs, mine, nil, nil); !isMergeDirection(err) {
			t.Errorf("a preview archiving the colleague's record = %v, want the merge_direction refusal", err)
		}
		if _, err := c.store.MergeContact(rep, contactID(theirs), contactID(mine), nil); !isMergeDirection(err) {
			t.Errorf("a merge archiving the colleague's record = %v, want the merge_direction refusal", err)
		}
		if _, err := c.store.PreviewMergeSurvivorship(c.previewer(merger), "contact", mine, theirs, nil, nil); err != nil {
			t.Fatalf("the preview refused giving one's own record away: %v", err)
		}
		survivor, err := c.store.MergeContact(rep, contactID(mine), contactID(theirs), nil)
		if err != nil {
			t.Fatalf("giving one's own record away was refused: %v", err)
		}
		if survivor.OwnerId == nil || ids.UUID(*survivor.OwnerId) != colleague {
			t.Errorf("the survivor's owner changed, want the colleague %s", colleague)
		}
		if n := c.mergedBy(t, merger, mine, theirs); n != 2 {
			t.Errorf("%d merge audit rows name the merger on the two records, want 2", n)
		}
		if got := c.e.WsScalar(t, `SELECT disposition FROM dedupe_candidate WHERE id = $1`, c.pairID(t, mine, theirs)); got != "merged" {
			t.Errorf("the filed pair reads %s, want merged", got)
		}
	}
}

// A rep who makes their own contact look like a colleague's gets a filed pair.
// Through it they may only give their own contact away.
func TestAManufacturedCollisionCannotArchiveTheColleaguesRecord(t *testing.T) {
	c := setupCases(t)
	rep := c.rep(c.e.Rep1)
	newContact := func(ctx context.Context, owner ids.UUID) ids.UUID {
		user := ids.From[ids.UserKind](owner)
		created, err := c.store.CreateContact(ctx, contacts.CreateContactInput{
			FullName: "Hedy Lamarr", Source: "manual", OwnerID: &user,
			Phones: []contacts.ContactPhoneInput{{Phone: "+493055599001", PhoneType: "work", IsPrimary: true, Position: 1}},
		})
		if err != nil {
			t.Fatalf("creating a contact: %v", err)
		}
		return ids.UUID(created.Id)
	}
	theirs := newContact(c.e.Admin(), c.e.Rep2)
	mine := newContact(rep, c.e.Rep1)
	c.pairID(t, mine, theirs)
	if _, err := c.store.MergeContact(rep, contactID(theirs), contactID(mine), nil); !isMergeDirection(err) {
		t.Fatalf("archiving the colleague's record through a made-up pair = %v, want the merge_direction refusal", err)
	}
	if n := c.e.WsCount(t, `SELECT count(*) FROM contact WHERE id = $1 AND archived_at IS NULL AND owner_id = $2`, theirs, c.e.Rep2); n != 1 {
		t.Error("the colleague's record is no longer live and theirs")
	}
}

// A pair re-homed onto a survivor by an earlier merge is open between two
// records nobody compared. It still only lets a rep give their own away.
func TestARehomedPairCannotArchiveTheColleaguesRecord(t *testing.T) {
	c := setupCases(t)
	newContact := func(name string, owner ids.UUID, phones ...string) ids.UUID {
		user := ids.From[ids.UserKind](owner)
		in := contacts.CreateContactInput{FullName: name, Source: "manual", OwnerID: &user}
		for i, phone := range phones {
			in.Phones = append(in.Phones, contacts.ContactPhoneInput{Phone: phone, PhoneType: "work", IsPrimary: i == 0, Position: i + 1})
		}
		created, err := c.store.CreateContact(c.e.Admin(), in)
		if err != nil {
			t.Fatalf("creating %s: %v", name, err)
		}
		return ids.UUID(created.Id)
	}
	mine := newContact("Ada Rehome", c.e.Rep1, "+493055599002")
	retired := newContact("Ada Rehome", c.e.Rep2, "+493055599003", "+493055599002")
	theirs := newContact("Zoltan Third", c.e.Rep2, "+493055599003")
	if n := c.e.WsCount(t, `SELECT count(*) FROM dedupe_candidate WHERE $1 IN (left_contact_id, right_contact_id)
		AND $2 IN (left_contact_id, right_contact_id)`, mine, theirs); n != 0 {
		t.Fatalf("the two ends were filed against each other %d time(s) before any merge", n)
	}
	if _, err := c.store.MergeContact(c.e.Admin(), contactID(retired), contactID(mine), nil); err != nil {
		t.Fatalf("the earlier merge: %v", err)
	}
	c.pairID(t, mine, theirs)
	if _, err := c.store.MergeContact(c.rep(c.e.Rep1), contactID(theirs), contactID(mine), nil); !isMergeDirection(err) {
		t.Errorf("archiving the colleague's record through a re-homed pair = %v, want the merge_direction refusal", err)
	}
}

func isMergeDirection(err error) bool {
	return errors.Is(err, apperrors.ErrPermissionDenied) && caseCode(err) == "merge_direction"
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
	if len(listed) != 1 || !listed[0].CanMerge || listed[0].CanDecide || listed[0].KeepID == nil || *listed[0].KeepID != queued[0] {
		t.Fatalf("the owner of one half lists %+v, want can_merge keeping the colleague's record and no can_decide", listed)
	}
	mine := queued[1]
	if _, err := c.store.DisposeDedupeCandidate(c.rep(c.e.Rep2), c.pairID(t, queued[0], queued[1]), "merge", &mine); !isMergeDirection(err) {
		t.Errorf("a queue merge keeping the merger's own record = %v, want the merge_direction refusal", err)
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
	if _, err := c.store.MergeContact(rep, contactID(dismissed[0]), contactID(dismissed[1]), nil); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("a merge of one's own record into a pair someone marked as two contacts = %v, want the bare conflict", err)
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
	stranger := c.unlikeCompany(t, c.e.Rep2)
	if _, err := c.store.MergeCompany(rep, ids.From[ids.CompanyKind](mine), ids.From[ids.CompanyKind](stranger), nil); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("a merge into a colleague's unfiled company = %v, want the bare conflict", err)
	}
	if _, err := c.store.MergeCompany(rep, ids.From[ids.CompanyKind](theirs), ids.From[ids.CompanyKind](mine), nil); !isMergeDirection(err) {
		t.Errorf("archiving the colleague's company = %v, want the merge_direction refusal", err)
	}
	survivor, err := c.store.MergeCompany(rep, ids.From[ids.CompanyKind](mine), ids.From[ids.CompanyKind](theirs), nil)
	if err != nil {
		t.Fatalf("the owner of one company giving it away was refused: %v", err)
	}
	if survivor.OwnerId == nil || ids.UUID(*survivor.OwnerId) != c.e.Rep2 {
		t.Error("the surviving company changed owner")
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

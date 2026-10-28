// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Contact duplicate cases over the real merge, consent and grant edges, acted
// on by ordinary own-scope reps. Here because the case merge keeps former
// owners' access through identity's grant writer, which contacts cannot import.

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/introductions"
	"github.com/margince/margince/backend/internal/platform/config/configtest"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// caseRepPerms is the seeded rep: contacts at own scope.
var caseRepPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"contact":               {Create: true, Read: true, Update: true},
		"relationship":          {Create: true, Read: true, Update: true},
		"installation_settings": {Read: true},
	},
	RowScope: principal.RowScopeOwn,
}

type caseEnv struct {
	e     *integration.Env
	store *contacts.Store
	// phone keeps each seeded case on a number of its own.
	phone int
}

func setupCases(t *testing.T) *caseEnv {
	t.Helper()
	e := integration.Setup(t)
	consentStore := consent.NewStore(e.DB())
	return &caseEnv{e: e, phone: 1000, store: contacts.NewStore(e.DB()).
		WithStopCarrier(consentStore).
		WithSatelliteCarriers(consentStore, introductions.NewStore(e.DB(), time.Now)).
		WithGrantCarrier(identity.NewServiceFor(e.DB())).
		WithParticipantRosterReader(activities.EnsureParticipantRosterVisibleTx).
		WithSettings(configtest.StoredOver(t, e.Pool, contacts.ConfigSettings()))}
}

// rep binds a full-seat own-scope rep.
func (c *caseEnv) rep(user ids.UUID) context.Context {
	ctx := c.e.As(user, nil, caseRepPerms)
	p, _ := principal.Actor(ctx)
	p.SeatType = principal.SeatFull
	return principal.WithActor(ctx, p)
}

func (c *caseEnv) setPolicy(t *testing.T, on bool) {
	t.Helper()
	if err := c.store.SetSharedMergeReviews(c.e.Admin(), on); err != nil {
		t.Fatalf("switching shared merge reviews: %v", err)
	}
}

// seedCase creates one contact per owner through the real create, all sharing
// one number, so each create files its pairs with the ones before it.
func (c *caseEnv) seedCase(t *testing.T, owners ...ids.UUID) []ids.UUID {
	t.Helper()
	c.phone++
	number := "+4930555" + itoa(c.phone)
	name := caseNames[c.phone%len(caseNames)]
	var members []ids.UUID
	for _, owner := range owners {
		user := ids.From[ids.UserKind](owner)
		created, err := c.store.CreateContact(c.e.Admin(), contacts.CreateContactInput{
			FullName: name, Source: "manual", OwnerID: &user,
			Phones: []contacts.ContactPhoneInput{{Phone: number, PhoneType: "work", IsPrimary: true, Position: 1}},
		})
		if err != nil {
			t.Fatalf("creating a duplicate: %v", err)
		}
		members = append(members, ids.UUID(created.Id))
	}
	got, err := c.store.GetContactCase(c.e.Admin(), members[0])
	if err != nil {
		t.Fatalf("reading the seeded case: %v", err)
	}
	if len(got.Members) != len(members) {
		t.Fatalf("the seeded case holds %d members, want %d", len(got.Members), len(members))
	}
	return members
}

// caseNames gives each seeded case a name no other case resembles, so the
// fuzzy name lane files no pair across two cases.
var caseNames = []string{"Ada Lovelace", "Grace Hopper", "Katherine Johnson", "Edsger Dijkstra", "Barbara Liskov"}

func itoa(n int) string {
	digits := []byte{}
	for ; n > 0; n /= 10 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
	}
	return string(digits)
}

func (c *caseEnv) read(t *testing.T, ctx context.Context, member ids.UUID) crmcontracts.ContactCase {
	t.Helper()
	got, err := c.store.GetContactCase(ctx, member)
	if err != nil {
		t.Fatalf("reading the case: %v", err)
	}
	return got
}

func caseDecision(read crmcontracts.ContactCase, members []ids.UUID, survivor, owner ids.UUID) contacts.ContactCaseDecision {
	return contacts.ContactCaseDecision{Revision: read.Revision, MemberIDs: members, SurvivorID: survivor, OwnerID: owner}
}

func caseCode(err error) string {
	var selection *contacts.CaseSelectionError
	if errors.As(err, &selection) {
		return selection.Code
	}
	type coded interface{ MessageFault() (string, string) }
	var fault coded
	if errors.As(err, &fault) {
		code, _ := fault.MessageFault()
		return code
	}
	return ""
}

func uuidsOf(in []openapi_types.UUID) []ids.UUID {
	out := make([]ids.UUID, 0, len(in))
	for _, id := range in {
		out = append(out, ids.UUID(id))
	}
	return out
}

func TestEveryOwnerSeesTheSameContactCase(t *testing.T) {
	c := setupCases(t)
	members := c.seedCase(t, c.e.Rep1, c.e.Rep2, c.e.Rep3)
	anchor := slices.MinFunc(members, func(a, b ids.UUID) int { return slices.Compare(a[:], b[:]) })

	for i, owner := range []ids.UUID{c.e.Rep1, c.e.Rep2, c.e.Rep3} {
		got := c.read(t, c.rep(owner), members[i])
		if ids.UUID(got.AnchorId) != anchor || len(got.Members) != 3 || len(got.Owners) != 3 {
			t.Errorf("owner %d reads anchor %s with %d members and %d owners, want %s, 3 and 3",
				i, got.AnchorId, len(got.Members), len(got.Owners), anchor)
		}
		page, _, err := c.store.ListContactCases(c.rep(owner), contacts.ContactCaseListInput{Scope: "mine"})
		if err != nil {
			t.Fatalf("listing owner %d's cases: %v", i, err)
		}
		if len(page) != 1 || ids.UUID(page[0].AnchorId) != anchor {
			t.Errorf("owner %d's own cases = %d, want the one case", i, len(page))
		}
	}
	unassigned, _, err := c.store.ListContactCases(c.e.Admin(), contacts.ContactCaseListInput{Scope: "unassigned"})
	if err != nil || len(unassigned) != 0 {
		t.Errorf("unassigned cases = %d (%v), want none: every member has a live owner", len(unassigned), err)
	}

	twice := c.seedCase(t, c.e.Rep1, c.e.Rep1)
	got := c.read(t, c.rep(c.e.Rep1), twice[0])
	if len(got.Owners) != 1 || len(got.Owners[0].MemberIds) != 2 {
		t.Errorf("one owner of two members reads %d owner entries, want one naming both", len(got.Owners))
	}
}

func TestACaseMergeOfColleaguesContactsNeedsThePolicy(t *testing.T) {
	c := setupCases(t)
	members := c.seedCase(t, c.e.Rep1, c.e.Rep2, c.e.Rep3)
	rep := c.rep(c.e.Rep1)

	off := c.read(t, rep, members[0])
	if off.MergeMode != crmcontracts.ContactCaseMergeModeNone || off.CanMerge {
		t.Fatalf("with the policy off a rep reads merge_mode %s, want none", off.MergeMode)
	}
	_, err := c.store.MergeContactCase(rep, members[0], caseDecision(off, members, members[1], c.e.Rep1))
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a merge of colleagues' contacts with the policy off = %v, want 403", err)
	}

	c.setPolicy(t, true)
	on := c.read(t, rep, members[0])
	if on.MergeMode != crmcontracts.ContactCaseMergeModeDelegated {
		t.Fatalf("with the policy on the owner reads merge_mode %s, want delegated", on.MergeMode)
	}
	out, err := c.store.MergeContactCase(rep, members[0], caseDecision(on, members, members[1], c.e.Rep1))
	if err != nil {
		t.Fatalf("the delegated merge: %v", err)
	}
	if !out.Committed || out.Mode != crmcontracts.ContactCaseOutcomeModeDelegated || len(out.MergedIds) != 2 {
		t.Fatalf("outcome = %+v, want a committed delegated merge of two", out)
	}
	if out.Survivor == nil || out.Survivor.OwnerId == nil || ids.UUID(*out.Survivor.OwnerId) != c.e.Rep1 {
		t.Errorf("the survivor is not owned by the chosen owner")
	}
	want := map[ids.UUID]string{c.e.Rep2: "write", c.e.Rep3: "write"}
	if got := sharesOf(out); !mapsEqual(got, want) {
		t.Errorf("shares = %v, want %v", got, want)
	}
	for owner, access := range want {
		if got := grantAccess(t, c.e, members[1], owner); got != access {
			t.Errorf("stored grant for %s = %q, want %q", owner, got, access)
		}
	}
}

func sharesOf(out crmcontracts.ContactCaseOutcome) map[ids.UUID]string {
	got := map[ids.UUID]string{}
	for _, share := range out.Shares {
		got[ids.UUID(share.UserId)] = string(share.Access)
	}
	return got
}

func mapsEqual[K comparable, V comparable](a, b map[K]V) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

func grantAccess(t *testing.T, e *integration.Env, record, user ids.UUID) string {
	t.Helper()
	if e.WsCount(t, `SELECT count(*) FROM record_grant WHERE record_type = 'contact' AND record_id = $1
		AND subject_type = 'user' AND subject_id = $2`, record, user) == 0 {
		return ""
	}
	return e.WsScalar(t, `SELECT access FROM record_grant WHERE record_type = 'contact' AND record_id = $1
		AND subject_type = 'user' AND subject_id = $2`, record, user)
}

func TestACaseMergeKeepsEachFormerOwnerAtTheirSeat(t *testing.T) {
	c := setupCases(t)
	owner := integration.OwnerConn(t)
	gone := ids.NewV7()
	if _, err := owner.Exec(context.Background(), `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Gone Rep')`,
		gone, "gone-"+gone.String()+"@case.test"); err != nil {
		t.Fatal(err)
	}
	members := c.seedCase(t, c.e.Rep1, c.e.Rep2, c.e.Rep3, gone)
	for _, stmt := range []string{
		`UPDATE app_user SET seat_type = 'read' WHERE id = $1`,
	} {
		if _, err := owner.Exec(context.Background(), stmt, c.e.Rep3); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := owner.Exec(context.Background(), `UPDATE app_user SET status = 'deactivated' WHERE id = $1`, gone); err != nil {
		t.Fatal(err)
	}
	c.setPolicy(t, true)
	rep := c.rep(c.e.Rep1)
	read := c.read(t, rep, members[0])
	out, err := c.store.MergeContactCase(rep, members[0], caseDecision(read, members, members[0], c.e.Rep1))
	if err != nil {
		t.Fatalf("merging the case: %v", err)
	}
	if got, want := sharesOf(out), map[ids.UUID]string{c.e.Rep2: "write", c.e.Rep3: "read"}; !mapsEqual(got, want) {
		t.Errorf("shares = %v, want %v: a read seat keeps read", got, want)
	}
	if grantAccess(t, c.e, members[0], c.e.Rep3) != "read" {
		t.Error("the read seat's stored grant is not read")
	}
	if len(out.Skipped) != 1 || ids.UUID(out.Skipped[0].UserId) != gone || out.Skipped[0].Reason != "inactive" {
		t.Errorf("skipped = %+v, want the departed owner as inactive", out.Skipped)
	}
	if grantAccess(t, c.e, members[0], gone) != "" {
		t.Error("a departed owner was granted access")
	}
}

func TestASubsetMergeLeavesTheRestAsAnOpenPairWithTheSurvivor(t *testing.T) {
	c := setupCases(t)
	members := c.seedCase(t, c.e.Rep1, c.e.Rep2, c.e.Rep3)
	c.setPolicy(t, true)
	rep := c.rep(c.e.Rep1)
	read := c.read(t, rep, members[0])
	out, err := c.store.MergeContactCase(rep, members[0], caseDecision(read, members[:2], members[0], c.e.Rep1))
	if err != nil {
		t.Fatalf("merging two of three: %v", err)
	}
	if got := sharesOf(out); !mapsEqual(got, map[ids.UUID]string{c.e.Rep2: "write"}) {
		t.Errorf("shares = %v, want only the selected colleague", got)
	}
	if grantAccess(t, c.e, members[0], c.e.Rep3) != "" {
		t.Error("the unselected member's owner was granted access")
	}
	residual := c.read(t, c.e.Admin(), members[2])
	if got := uuidsOf(memberIDs(residual)); len(got) != 2 || !slices.Contains(got, members[0]) {
		t.Errorf("the residual case = %v, want the third member and the survivor", got)
	}
	if residual.Edges[0].Status != crmcontracts.ContactCaseEdgeStatusOpen {
		t.Errorf("the residual pair is %s, want open", residual.Edges[0].Status)
	}
}

func memberIDs(c crmcontracts.ContactCase) []openapi_types.UUID {
	out := make([]openapi_types.UUID, 0, len(c.Members))
	for _, m := range c.Members {
		out = append(out, m.Id)
	}
	return out
}

func TestACaseMergeRefusesASelectionTheCaseCannotTake(t *testing.T) {
	c := setupCases(t)
	members := c.seedCase(t, c.e.Rep1, c.e.Rep1, c.e.Rep1)
	stranger := c.seedCase(t, c.e.Rep1, c.e.Rep1)
	rep := c.rep(c.e.Rep1)

	// The nightly pass files the pair the creates skipped, so the three form a
	// triangle and one pair can be dismissed without leaving the case.
	if _, err := c.store.ScanChangedForDuplicates(machineCtx(c.e)); err != nil {
		t.Fatalf("scanning for duplicates: %v", err)
	}
	read := c.read(t, rep, members[0])
	if len(read.Edges) != 3 {
		t.Fatalf("the case holds %d pairs, want the triangle of 3", len(read.Edges))
	}
	pair := read.Edges[0]
	if _, err := c.store.DisposeDedupeCandidate(rep, ids.UUID(pair.PairId), "not_a_duplicate", nil); err != nil {
		t.Fatalf("dismissing a pair: %v", err)
	}
	read = c.read(t, rep, members[0])
	cases := []struct {
		name   string
		member []ids.UUID
		owner  ids.UUID
		want   string
	}{
		{"a dismissed pair between two selected", members, c.e.Rep1, "dismissed_pair_selected"},
		{"a contact outside the case", []ids.UUID{members[0], stranger[0]}, c.e.Rep1, "not_connected"},
		{"an owner of no selected member", []ids.UUID{ids.UUID(pair.LeftId), otherThan(members, pair)}, c.e.Rep2, "owner_not_eligible"},
	}
	for _, tc := range cases {
		_, err := c.store.MergeContactCase(rep, members[0], caseDecision(read, tc.member, tc.member[0], tc.owner))
		if got := caseCode(err); got != tc.want {
			t.Errorf("%s = %v (code %q), want %s", tc.name, err, got, tc.want)
		}
	}
}

func otherThan(members []ids.UUID, pair crmcontracts.ContactCaseEdge) ids.UUID {
	for _, id := range members {
		if id != ids.UUID(pair.LeftId) && id != ids.UUID(pair.RightId) {
			return id
		}
	}
	return ids.Nil
}

func TestUndoingADismissalReshapesTheCase(t *testing.T) {
	c := setupCases(t)
	members := c.seedCase(t, c.e.Rep1, c.e.Rep1, c.e.Rep1)
	rep := c.rep(c.e.Rep1)
	third := members[2]
	var dismissed []ids.UUID
	for _, edge := range c.read(t, rep, members[0]).Edges {
		if ids.UUID(edge.LeftId) == third || ids.UUID(edge.RightId) == third {
			if _, err := c.store.DisposeDedupeCandidate(rep, ids.UUID(edge.PairId), "not_a_duplicate", nil); err != nil {
				t.Fatal(err)
			}
			dismissed = append(dismissed, ids.UUID(edge.PairId))
		}
	}
	if got := c.read(t, rep, members[0]); len(got.Members) != 2 {
		t.Fatalf("after dismissing the third member's pairs the case holds %d members, want 2", len(got.Members))
	}
	if _, err := c.store.UndoDedupeDisposition(rep, dismissed[0]); err != nil {
		t.Fatal(err)
	}
	if got := c.read(t, rep, members[0]); len(got.Members) != 3 {
		t.Errorf("after the undo the case holds %d members, want 3 again", len(got.Members))
	}
}

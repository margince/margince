// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// "Who can see this record" (GET /contacts/{id}/access, /companies/{id}/access)
// against records, roles, teams and shares written through the product's own
// writers. The first test is the one that holds the feature: for every live
// member, being listed must equal the contact or company read path admitting
// them, and can_change must equal the edit path admitting them.

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// accessWorld is the fixture: Rep1 and Rep2 share Team1, Rep3 sits in Team2,
// AdminUser is in no team.
//
//   - Rep1, Rep3: rep (own scope). Rep2: manager (team scope). AdminUser: admin.
//   - reader: the rep role on a READ seat, in Team1, owning a contact of its own.
//   - rep4: rep, in no team. roleless: live, holding no role at all.
//   - gone: a rep in Team1 who owns a contact and was then deactivated.
//
// Records: a workspace contact Rep1 owns; the reader's contact; a contact Rep3
// made private and shared with Rep1 (read), Team1 (write), the admin (read)
// and Rep2 (a share that has lapsed); a workspace company Rep3 owns; a company
// nobody owns; an archived contact; the deactivated member's contact.
type accessWorld struct {
	e                *Env
	directory        *identity.Service
	reads            compose.RecordAccessReads
	reader, roleless ids.UUID
	rep4, gone       ids.UUID
	workspaceContact ids.UUID
	readerContact    ids.UUID
	privateContact   ids.UUID
	ownedCompany     ids.UUID
	ownerlessCompany ids.UUID
	archivedContact  ids.UUID
	goneContact      ids.UUID
}

func setupAccessWorld(t *testing.T) *accessWorld {
	t.Helper()
	e := Setup(t)
	w := &accessWorld{e: e, directory: identity.NewService(e.Pool), reads: compose.NewRecordAccessReads(e.Pool)}
	w.reader, w.roleless, w.rep4, w.gone = ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7()
	e.WsExec(t, `INSERT INTO app_user (id, email, display_name, seat_type) VALUES
		($1, 'reader@access.test', 'Reader', 'full'), ($2, 'roleless@access.test', 'Roleless', 'full'),
		($3, 'rep4@access.test', 'Rep Four', 'full'), ($4, 'gone@access.test', 'Gone', 'full')`,
		w.reader, w.roleless, w.rep4, w.gone)
	for _, user := range []ids.UUID{w.reader, w.gone} {
		w.setTeam(t, e.Team1, user, true)
	}
	e.GrantRole(t, e.Rep1, "rep")
	e.GrantRole(t, e.Rep2, "manager")
	e.GrantRole(t, e.Rep3, "rep")
	e.GrantRole(t, e.AdminUser, "admin")
	e.GrantRole(t, w.reader, "rep")
	e.GrantRole(t, w.rep4, "rep")
	e.GrantRole(t, w.gone, "rep")

	w.workspaceContact = e.SeedContact(t, "Open Contact", &e.Rep1)
	w.readerContact = e.SeedContact(t, "Reader's Contact", &w.reader)
	// A read seat owns a record by being DOWNGRADED to one, never by being
	// handed it: a create refuses a read seat as owner, the same way a
	// handover does. The record outliving the seat change is the state these
	// cases are about, and the only way to reach it.
	e.WsExec(t, `UPDATE app_user SET seat_type = 'read' WHERE id = $1`, w.reader)
	w.privateContact = w.seedPrivateContact(t, &e.Rep3)
	w.ownedCompany = e.SeedCompany(t, "Owned GmbH", &e.Rep3)
	w.ownerlessCompany = e.SeedCompany(t, "Nobody's AG", &e.Rep3)
	if _, err := e.Contacts.UpdateCompany(w.as(t, e.Rep3), ids.From[ids.CompanyKind](w.ownerlessCompany),
		contacts.UpdateCompanyInput{Clear: []string{"owner_id"}}); err != nil {
		t.Fatalf("releasing the company's owner: %v", err)
	}
	w.archivedContact = e.SeedContact(t, "Gone Contact", &e.Rep1)
	if _, err := e.Contacts.ArchiveContact(w.as(t, e.AdminUser), ids.From[ids.ContactKind](w.archivedContact), nil); err != nil {
		t.Fatalf("archiving the contact: %v", err)
	}
	w.goneContact = e.SeedContact(t, "Left Behind", &w.gone)
	if err := w.directory.DeactivateUser(e.Admin(), w.adminIdentity(),
		identity.DeactivateUserInput{UserID: ids.From[ids.UserKind](w.gone)}); err != nil {
		t.Fatalf("deactivating the owner: %v", err)
	}
	return w
}

// seedPrivateContact is a contact made private by its owner and shared through
// the real writer: Rep1 (read), Team1 (write), the admin (read), and Rep2 on a
// share that has since lapsed.
func (w *accessWorld) seedPrivateContact(t *testing.T, ownerID *ids.UUID) ids.UUID {
	t.Helper()
	e := w.e
	id := e.SeedContact(t, "Private Contact", ownerID)
	owner := w.as(t, *ownerID)
	private := string(crmcontracts.ContactVisibilityOwner)
	if _, err := e.Contacts.UpdateContact(owner, ids.From[ids.ContactKind](id),
		contacts.UpdateContactInput{Visibility: &private}); err != nil {
		t.Fatalf("making the contact private: %v", err)
	}
	soon := time.Now().Add(time.Hour)
	for _, share := range []identity.CreateGrantInput{
		{SubjectType: "user", SubjectID: e.Rep1, Access: "read"},
		{SubjectType: "team", SubjectID: e.Team1, Access: "write"},
		{SubjectType: "user", SubjectID: e.AdminUser, Access: "read"},
		{SubjectType: "user", SubjectID: e.Rep2, Access: "read", ExpiresAt: &soon},
	} {
		share.RecordType, share.RecordID = "contact", id
		if _, err := w.directory.CreateRecordGrant(owner, share); err != nil {
			t.Fatalf("sharing the private contact with %s %s: %v", share.SubjectType, share.SubjectID, err)
		}
	}
	e.WsExec(t, `UPDATE record_grant SET expires_at = now() - interval '1 minute'
		WHERE record_id = $1 AND subject_id = $2`, id, e.Rep2)
	return id
}

func (w *accessWorld) adminIdentity() identity.Identity {
	return identity.Identity{
		UserID: ids.From[ids.UserKind](w.e.AdminUser), WorkspaceID: ids.From[ids.WorkspaceKind](w.e.WS),
		Roles: []string{"admin"}, SeatType: string(principal.SeatFull), Permissions: AdminPerms,
	}
}

// setTeam moves a member on or off a team the way an administrator does.
func (w *accessWorld) setTeam(t *testing.T, team, user ids.UUID, on bool) {
	t.Helper()
	if err := w.directory.SetTeamMember(w.e.Admin(), w.adminIdentity(), team, user, on); err != nil {
		t.Fatalf("setting %s on team %s to %v: %v", user, team, on, err)
	}
}

// as binds a member the way the session door does: their live grants, teams
// and seat, read in one snapshot.
func (w *accessWorld) as(t *testing.T, user ids.UUID) context.Context {
	t.Helper()
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), w.e.WS), ids.NewV7())
	p, err := authz.MemberPrincipal(ctx, w.directory, w.e.WS, user)
	if err != nil {
		t.Fatalf("resolving member %s: %v", user, err)
	}
	return principal.WithActor(ctx, p)
}

// answer walks every page of the answer; the first page carries the counts.
func (w *accessWorld) answer(t *testing.T, caller ids.UUID, table string, id ids.UUID) (crmcontracts.RecordAccess, map[ids.UUID]crmcontracts.RecordAccessMember) {
	t.Helper()
	out := map[ids.UUID]crmcontracts.RecordAccessMember{}
	var first crmcontracts.RecordAccess
	var cursor *string
	limit := 2
	for {
		page, err := w.reads.Read(w.as(t, caller), table, id, cursor, &limit)
		if err != nil {
			t.Fatalf("reading who can see %s %s: %v", table, id, err)
		}
		if cursor == nil {
			first = page
		}
		for _, m := range page.Data {
			if _, twice := out[ids.UUID(m.UserId)]; twice {
				t.Fatalf("member %s served on two pages", m.UserId)
			}
			out[ids.UUID(m.UserId)] = m
		}
		if !page.Page.HasMore {
			return first, out
		}
		cursor = page.Page.NextCursor
	}
}

func (w *accessWorld) everyone(t *testing.T, caller ids.UUID, table string, id ids.UUID) map[ids.UUID]crmcontracts.RecordAccessMember {
	t.Helper()
	_, members := w.answer(t, caller, table, id)
	return members
}

func (w *accessWorld) liveMembers(t *testing.T) []ids.UUID {
	t.Helper()
	var out []ids.UUID
	if err := database.WithWorkspaceTx(w.e.Admin(), w.e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `SELECT id FROM app_user WHERE `+identity.LiveMemberSQL(""))
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		return err
	}); err != nil {
		t.Fatalf("listing live members: %v", err)
	}
	return out
}

// pathAdmits runs one real path and reads its refusal as "no".
func pathAdmits(t *testing.T, what string, err error) bool {
	t.Helper()
	switch {
	case err == nil:
		return true
	case errors.Is(err, apperrors.ErrNotFound), errors.Is(err, apperrors.ErrPermissionDenied),
		errors.Is(err, apperrors.ErrSeatTierInsufficient):
		return false
	default:
		t.Fatalf("%s: %v", what, err)
		return false
	}
}

// opensAndChanges asks the record's own read path and its own edit path, the
// latter with an empty patch so nothing is written.
func (w *accessWorld) opensAndChanges(ctx context.Context, t *testing.T, table string, id ids.UUID) (bool, bool) {
	t.Helper()
	if table == "contact" {
		cid := ids.From[ids.ContactKind](id)
		_, readErr := w.e.Contacts.GetContact(ctx, cid, storekit.IncludeArchived)
		_, editErr := w.e.Contacts.UpdateContact(ctx, cid, contacts.UpdateContactInput{})
		return pathAdmits(t, "reading the contact", readErr), pathAdmits(t, "editing the contact", editErr)
	}
	cid := ids.From[ids.CompanyKind](id)
	_, readErr := w.e.Contacts.GetCompany(ctx, cid, storekit.IncludeArchived)
	_, editErr := w.e.Contacts.UpdateCompany(ctx, cid, contacts.UpdateCompanyInput{})
	return pathAdmits(t, "reading the company", readErr), pathAdmits(t, "editing the company", editErr)
}

// The agreement is asked of the administrator's answer, the one that judges
// every member with their teams: a rep's answer leaves teams out on purpose.
func TestWhoCanSeeARecordAgreesWithItsReadAndEditPaths(t *testing.T) {
	w := setupAccessWorld(t)
	for _, rec := range []struct {
		name  string
		table string
		id    ids.UUID
	}{
		{"a workspace contact", "contact", w.workspaceContact},
		{"a contact a read seat owns", "contact", w.readerContact},
		{"a private contact with shares", "contact", w.privateContact},
		{"a company with an owner", "company", w.ownedCompany},
		{"a company nobody owns", "company", w.ownerlessCompany},
		{"an archived contact", "contact", w.archivedContact},
		{"a contact whose owner was deactivated", "contact", w.goneContact},
	} {
		answer, listed := w.answer(t, w.e.AdminUser, rec.table, rec.id)
		if !answer.Detail || answer.TeamAccessCount != 0 {
			t.Fatalf("%s: the administrator's answer has detail=%v and %d members left out", rec.name, answer.Detail, answer.TeamAccessCount)
		}
		for _, member := range w.liveMembers(t) {
			opens, changes := w.opensAndChanges(w.as(t, member), t, rec.table, rec.id)
			got, isListed := listed[member]
			if isListed != opens {
				t.Errorf("%s: member %s listed=%v but the read path admits=%v", rec.name, member, isListed, opens)
				continue
			}
			if isListed && got.CanChange != changes {
				t.Errorf("%s: member %s can_change=%v but the edit path admits=%v", rec.name, member, got.CanChange, changes)
			}
			if isListed && len(got.ReadReasons) == 0 {
				t.Errorf("%s: member %s is listed with no reason", rec.name, member)
			}
			if got.CanChange && len(got.ChangeReasons) == 0 {
				t.Errorf("%s: member %s can change it with no reason", rec.name, member)
			}
		}
	}
}

func TestAWorkspaceContactListsEveryReaderAndOnlyThem(t *testing.T) {
	w := setupAccessWorld(t)
	e := w.e
	listed := w.everyone(t, e.AdminUser, "contact", w.workspaceContact)
	if _, ok := listed[w.roleless]; ok {
		t.Error("a member whose role grants no contact read is listed")
	}
	reader, ok := listed[w.reader]
	if !ok || reader.CanChange {
		t.Errorf("the read seat: listed=%v can_change=%v, want listed and unable to change", ok, reader.CanChange)
	}
	owner := listed[e.Rep1]
	if owner.Group != crmcontracts.RecordAccessMemberGroupOwner || !hasCode(owner.ChangeReasons, crmcontracts.RecordAccessReasonCodeOwner) {
		t.Errorf("the owner is grouped %q with change reasons %v", owner.Group, owner.ChangeReasons)
	}
	lead := listed[e.Rep2]
	if !lead.CanChange || !hasCode(lead.ChangeReasons, crmcontracts.RecordAccessReasonCodeSameTeamAsOwner) {
		t.Errorf("the owner's team lead: can_change=%v reasons %v, want same_team_as_owner", lead.CanChange, lead.ChangeReasons)
	}
	if colleague := listed[e.Rep3]; colleague.Group != crmcontracts.RecordAccessMemberGroupEveryone || colleague.CanChange {
		t.Errorf("a rep in another team: group %q can_change=%v, want everyone and read only", colleague.Group, colleague.CanChange)
	}
}

// The deactivated owner keeps their team membership, and the team-scope write
// arm reads that membership: the lead can still change the contact, and the
// answer says why.
func TestADeactivatedOwnersTeamLeadStillHasAReason(t *testing.T) {
	w := setupAccessWorld(t)
	lead := w.everyone(t, w.e.AdminUser, "contact", w.goneContact)[w.e.Rep2]
	if !lead.CanChange || !hasCode(lead.ChangeReasons, crmcontracts.RecordAccessReasonCodeSameTeamAsOwner) {
		t.Errorf("the deactivated owner's team lead: can_change=%v reasons %v, want same_team_as_owner", lead.CanChange, lead.ChangeReasons)
	}
}

func TestARecordAReadSeatOwnsOpensForItButDoesNotChange(t *testing.T) {
	w := setupAccessWorld(t)
	owner, ok := w.everyone(t, w.e.Rep1, "contact", w.readerContact)[w.reader]
	if !ok || owner.Group != crmcontracts.RecordAccessMemberGroupOwner {
		t.Fatalf("the read seat's own contact: listed=%v group %q, want listed as owner", ok, owner.Group)
	}
	if owner.CanChange || len(owner.ChangeReasons) != 0 {
		t.Errorf("a read seat can change its own contact (reasons %v): the seat ceiling holds whatever the role grants", owner.ChangeReasons)
	}
	you, err := w.reads.Read(w.as(t, w.reader), "contact", w.readerContact, nil, nil)
	if err != nil {
		t.Fatalf("the read seat reading its own contact: %v", err)
	}
	if you.You.CanChange || !hasCode(you.You.ReadReasons, crmcontracts.RecordAccessReasonCodeOwner) {
		t.Errorf("the read seat's own line: can_change=%v read reasons %v", you.You.CanChange, you.You.ReadReasons)
	}
}

func TestAPrivateContactListsItsOwnerAndLiveSharesOnly(t *testing.T) {
	w := setupAccessWorld(t)
	e := w.e
	answer, listed := w.answer(t, e.AdminUser, "contact", w.privateContact)
	if got := listed[e.AdminUser]; got.Group != crmcontracts.RecordAccessMemberGroupShared {
		t.Errorf("the admin reads it only through their share, and is grouped %q", got.Group)
	}
	if _, ok := listed[e.Rep3]; !ok {
		t.Error("the owner is not listed")
	}
	if _, ok := listed[w.rep4]; ok {
		t.Error("a rep with no share and no team is listed on a private contact")
	}
	if lapsed, ok := listed[e.Rep2]; ok && hasCode(lapsed.ReadReasons, crmcontracts.RecordAccessReasonCodeUserShare) {
		t.Error("a lapsed share still lists its holder by that share")
	}
	if got := listed[e.Rep1]; got.Group != crmcontracts.RecordAccessMemberGroupShared || !got.CanChange {
		t.Errorf("Rep1 (read share, and Team1's write share): group %q can_change=%v", got.Group, got.CanChange)
	}
	if got, ok := listed[w.reader]; !ok || got.Group != crmcontracts.RecordAccessMemberGroupTeamShared || got.CanChange {
		t.Errorf("the read seat in Team1: listed=%v group %q can_change=%v, want team_shared and unable to change", ok, got.Group, got.CanChange)
	}
	if answer.Visibility != crmcontracts.RecordAccessVisibilityOwner || answer.RefreshAt != nil || answer.GroupCounts.Everyone != 0 {
		t.Errorf("visibility %q, refresh_at %v, %d as everyone; want owner, no pending expiry, none", answer.Visibility, answer.RefreshAt, answer.GroupCounts.Everyone)
	}
}

func TestACompanyNobodyOwnsHasNoOwnerReasons(t *testing.T) {
	w := setupAccessWorld(t)
	for member, got := range w.everyone(t, w.e.AdminUser, "company", w.ownerlessCompany) {
		for _, reason := range slices.Concat(got.ReadReasons, got.ChangeReasons) {
			if reason.Code == crmcontracts.RecordAccessReasonCodeOwner || reason.Code == crmcontracts.RecordAccessReasonCodeSameTeamAsOwner {
				t.Errorf("member %s holds %q on a company nobody owns", member, reason.Code)
			}
		}
	}
}

// A rep can read the record's shares and the roster, and neither says who is
// in which team. Their answer must not either: moving a member between teams
// leaves every row they are shown exactly as it was.
func TestAColleagueOutsideMemberAdministrationCannotTellWhoIsInATeam(t *testing.T) {
	w := setupAccessWorld(t)
	e := w.e
	if _, err := w.directory.CreateRecordGrant(w.as(t, e.Rep1), identity.CreateGrantInput{
		RecordType: "contact", RecordID: w.workspaceContact, SubjectType: "team", SubjectID: e.Team2, Access: "write",
	}); err != nil {
		t.Fatalf("sharing with Team2: %v", err)
	}
	before, beforeRows := w.answer(t, e.Rep1, "contact", w.workspaceContact)
	w.setTeam(t, e.Team2, e.Rep3, false)
	w.setTeam(t, e.Team2, w.rep4, true)
	after, afterRows := w.answer(t, e.Rep1, "contact", w.workspaceContact)

	if !reflect.DeepEqual(beforeRows, afterRows) || before.GroupCounts != after.GroupCounts ||
		before.TeamAccessCount != after.TeamAccessCount {
		t.Errorf("a rep's answer changed when Team2 swapped Rep3 for Rep4, so it says who is in the team:\nbefore %+v\nafter  %+v", beforeRows, afterRows)
	}
	// Rep2 through team scope and Rep3 through Team2's share: counted, not named.
	if before.Detail || before.TeamAccessCount != 2 {
		t.Errorf("detail=%v, %d left out; want a rep's answer counting the two members a team lets edit", before.Detail, before.TeamAccessCount)
	}
	for member, got := range beforeRows {
		if member == e.Rep1 {
			continue
		}
		for _, reason := range slices.Concat(got.ReadReasons, got.ChangeReasons) {
			if reason.Code == crmcontracts.RecordAccessReasonCodeTeamShare || reason.Code == crmcontracts.RecordAccessReasonCodeSameTeamAsOwner || reason.TeamId != nil {
				t.Errorf("a rep is shown a team reason for %s: %+v", member, reason)
			}
		}
	}
}

func TestOnlyAMemberAdministratorSeesRolesAndTeams(t *testing.T) {
	w := setupAccessWorld(t)
	e := w.e
	for member, got := range w.everyone(t, e.Rep3, "contact", w.privateContact) {
		if got.Roles != nil {
			t.Errorf("a rep is shown %s's roles", member)
		}
	}
	reader := w.everyone(t, e.AdminUser, "contact", w.privateContact)[w.reader]
	if reader.Roles == nil || !slices.Contains(*reader.Roles, "rep") {
		t.Errorf("the admin is shown roles %v for the read seat, want rep", reader.Roles)
	}
	if !slices.ContainsFunc(reader.ReadReasons, func(r crmcontracts.RecordAccessReason) bool {
		return r.Code == crmcontracts.RecordAccessReasonCodeTeamShare && r.TeamId != nil && ids.UUID(*r.TeamId) == e.Team1
	}) {
		t.Errorf("the admin is not shown the team behind the read seat's share: %v", reader.ReadReasons)
	}
}

func TestACallerWhoCannotOpenTheRecordLearnsNothing(t *testing.T) {
	w := setupAccessWorld(t)
	e := w.e
	private := string(crmcontracts.ContactVisibilityOwner)
	id := e.SeedContact(t, "Nobody Else's", &e.Rep1)
	if _, err := e.Contacts.UpdateContact(w.as(t, e.Rep1), ids.From[ids.ContactKind](id),
		contacts.UpdateContactInput{Visibility: &private}); err != nil {
		t.Fatalf("making the contact private: %v", err)
	}
	_, err := w.reads.Read(w.as(t, e.AdminUser), "contact", id, nil, nil)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound: an admin without a share cannot tell the private contact exists", err)
	}
}

func TestTheAnswerPagesWithAnExactTotal(t *testing.T) {
	w := setupAccessWorld(t)
	limit := 2
	first, err := w.reads.Read(w.as(t, w.e.AdminUser), "contact", w.workspaceContact, nil, &limit)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	all := w.everyone(t, w.e.AdminUser, "contact", w.workspaceContact)
	if first.Page.Total == nil || *first.Page.Total != len(all) || len(first.Data) != limit || !first.Page.HasMore {
		t.Fatalf("first page: %d rows, total %v, has_more %v; want %d rows of %d", len(first.Data), first.Page.Total, first.Page.HasMore, limit, len(all))
	}
	counts := first.GroupCounts
	if counts.Owner+counts.Shared+counts.TeamShared+counts.Everyone != len(all) {
		t.Errorf("group counts %+v do not add up to %d members", counts, len(all))
	}
}

// More members than one admission statement judges: each keeps their own
// verdict across the statement boundary, and the newest member, who owns the
// contact, is on a later page and still in the owner group.
func TestEveryMemberKeepsTheirOwnVerdictPastOneStatement(t *testing.T) {
	w := setupAccessWorld(t)
	e := w.e
	e.WsExec(t, `INSERT INTO app_user (id, email, display_name, seat_type, created_at)
		SELECT gen_random_uuid(), 'm' || g || '@many.test', 'Member ' || g, 'full', now() + g * interval '1 second'
		  FROM generate_series(1, 150) g`)
	e.WsExec(t, `INSERT INTO role_assignment (role_id, user_id)
		SELECT r.id, u.id FROM app_user u JOIN role r ON r.key = 'rep' WHERE u.email LIKE '%@many.test'`)
	newest, err := ids.Parse(e.WsScalar(t, `SELECT id::text FROM app_user WHERE email = 'm150@many.test'`))
	if err != nil {
		t.Fatal(err)
	}
	contact := e.SeedContact(t, "Newest Owns It", &newest)

	limit := 50
	first, err := w.reads.Read(w.as(t, e.AdminUser), "contact", contact, nil, &limit)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if slices.ContainsFunc(first.Data, func(m crmcontracts.RecordAccessMember) bool { return ids.UUID(m.UserId) == newest }) {
		t.Fatal("the newest member is on the first page, so this does not reach a later one")
	}
	listed := w.everyone(t, e.AdminUser, "contact", contact)
	owner := listed[newest]
	if owner.Group != crmcontracts.RecordAccessMemberGroupOwner || !owner.CanChange {
		t.Errorf("the newest member owns the contact and is grouped %q can_change=%v", owner.Group, owner.CanChange)
	}
	changers := 0
	for _, m := range listed {
		if m.CanChange {
			changers++
		}
	}
	// Only the owner and the admin can change it; every other member reads it.
	if changers != 2 || first.CanChangeCount != 2 {
		t.Errorf("%d listed can change and %d counted, want 2: a verdict crossed a statement boundary", changers, first.CanChangeCount)
	}
}

func hasCode(reasons []crmcontracts.RecordAccessReason, code crmcontracts.RecordAccessReasonCode) bool {
	return slices.ContainsFunc(reasons, func(r crmcontracts.RecordAccessReason) bool { return r.Code == code })
}

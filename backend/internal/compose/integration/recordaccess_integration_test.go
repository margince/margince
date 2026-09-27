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
//   - roleless: a live member holding no role, so no contact read at all.
//
// Records: a workspace contact Rep1 owns; one the reader owns; a contact Rep3 made private and
// shared with Rep1 (read), Team1 (write) and Rep2 (a share that has lapsed); a
// workspace company Rep3 owns; a company nobody owns; an archived contact.
type accessWorld struct {
	e                *Env
	directory        *identity.Service
	reads            compose.RecordAccessReads
	reader, roleless ids.UUID
	workspaceContact ids.UUID
	readerContact    ids.UUID
	privateContact   ids.UUID
	ownedCompany     ids.UUID
	ownerlessCompany ids.UUID
	archivedContact  ids.UUID
}

func setupAccessWorld(t *testing.T) *accessWorld {
	t.Helper()
	e := Setup(t)
	w := &accessWorld{e: e, directory: identity.NewService(e.Pool), reads: compose.NewRecordAccessReads(e.Pool)}
	w.reader, w.roleless = ids.NewV7(), ids.NewV7()
	e.WsExec(t, `INSERT INTO app_user (id, email, display_name, seat_type) VALUES
		($1, 'reader@access.test', 'Reader', 'read'), ($2, 'roleless@access.test', 'Roleless', 'full')`,
		w.reader, w.roleless)
	e.WsExec(t, `INSERT INTO team_membership (team_id, user_id) VALUES ($1, $2)`, e.Team1, w.reader)
	e.GrantRole(t, e.Rep1, "rep")
	e.GrantRole(t, e.Rep2, "manager")
	e.GrantRole(t, e.Rep3, "rep")
	e.GrantRole(t, e.AdminUser, "admin")
	e.GrantRole(t, w.reader, "rep")

	w.workspaceContact = e.SeedContact(t, "Open Contact", &e.Rep1)
	w.readerContact = e.SeedContact(t, "Reader's Contact", &w.reader)
	w.privateContact = w.seedPrivateContact(t)
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
	return w
}

func (w *accessWorld) seedPrivateContact(t *testing.T) ids.UUID {
	t.Helper()
	e := w.e
	id := e.SeedContact(t, "Private Contact", &e.Rep3)
	owner := w.as(t, e.Rep3)
	private := string(crmcontracts.ContactVisibilityOwner)
	if _, err := e.Contacts.UpdateContact(owner, ids.From[ids.ContactKind](id),
		contacts.UpdateContactInput{Visibility: &private}); err != nil {
		t.Fatalf("making the contact private: %v", err)
	}
	soon := time.Now().Add(time.Hour)
	for _, share := range []identity.CreateGrantInput{
		{SubjectType: "user", SubjectID: e.Rep1, Access: "read"},
		{SubjectType: "team", SubjectID: e.Team1, Access: "write"},
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

// everyone walks every page of the answer.
func (w *accessWorld) everyone(t *testing.T, caller ids.UUID, table string, id ids.UUID) map[ids.UUID]crmcontracts.RecordAccessMember {
	t.Helper()
	out := map[ids.UUID]crmcontracts.RecordAccessMember{}
	var cursor *string
	limit := 2
	for {
		page, err := w.reads.Read(w.as(t, caller), table, id, cursor, &limit)
		if err != nil {
			t.Fatalf("reading who can see %s %s: %v", table, id, err)
		}
		for _, m := range page.Data {
			if _, twice := out[ids.UUID(m.UserId)]; twice {
				t.Fatalf("member %s served on two pages", m.UserId)
			}
			out[ids.UUID(m.UserId)] = m
		}
		if !page.Page.HasMore {
			return out
		}
		cursor = page.Page.NextCursor
	}
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
func (w *accessWorld) opensAndChanges(t *testing.T, ctx context.Context, table string, id ids.UUID) (bool, bool) {
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

func TestWhoCanSeeARecordAgreesWithItsReadAndEditPaths(t *testing.T) {
	w := setupAccessWorld(t)
	e := w.e
	for _, rec := range []struct {
		name   string
		table  string
		id     ids.UUID
		caller ids.UUID
	}{
		{"a workspace contact", "contact", w.workspaceContact, e.Rep1},
		{"a contact a read seat owns", "contact", w.readerContact, e.Rep1},
		{"a private contact with shares", "contact", w.privateContact, e.Rep3},
		{"a company with an owner", "company", w.ownedCompany, e.AdminUser},
		{"a company nobody owns", "company", w.ownerlessCompany, e.Rep1},
		{"an archived contact", "contact", w.archivedContact, e.Rep1},
	} {
		listed := w.everyone(t, rec.caller, rec.table, rec.id)
		for _, member := range w.liveMembers(t) {
			opens, changes := w.opensAndChanges(t, w.as(t, member), rec.table, rec.id)
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
	listed := w.everyone(t, e.Rep1, "contact", w.workspaceContact)
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
	listed := w.everyone(t, e.Rep3, "contact", w.privateContact)
	if _, ok := listed[e.AdminUser]; ok {
		t.Error("an admin with no share is listed on a private contact")
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
	answer, err := w.reads.Read(w.as(t, e.Rep3), "contact", w.privateContact, nil, nil)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if answer.Visibility != crmcontracts.RecordAccessVisibilityOwner || answer.RefreshAt != nil {
		t.Errorf("visibility %q refresh_at %v, want owner and no pending expiry", answer.Visibility, answer.RefreshAt)
	}
	if answer.GroupCounts.Everyone != 0 {
		t.Errorf("a private contact counts %d members as everyone", answer.GroupCounts.Everyone)
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

func TestOnlyAMemberAdministratorSeesRolesAndTeams(t *testing.T) {
	w := setupAccessWorld(t)
	e := w.e
	asRep := w.everyone(t, e.Rep3, "contact", w.privateContact)
	for member, got := range asRep {
		if got.Roles != nil {
			t.Errorf("a rep is shown %s's roles", member)
		}
		for _, reason := range got.ReadReasons {
			if reason.TeamId != nil || reason.TeamName != nil {
				t.Errorf("a rep is shown which team shares with %s", member)
			}
		}
	}
	if _, err := w.directory.CreateRecordGrant(w.as(t, e.Rep3), identity.CreateGrantInput{
		RecordType: "contact", RecordID: w.privateContact, SubjectType: "user", SubjectID: e.AdminUser, Access: "read",
	}); err != nil {
		t.Fatalf("sharing with the admin: %v", err)
	}
	asAdmin := w.everyone(t, e.AdminUser, "contact", w.privateContact)
	reader := asAdmin[w.reader]
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
	_, err := w.reads.Read(w.as(t, w.e.AdminUser), "contact", w.privateContact, nil, nil)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound: an admin without a share cannot tell the private contact exists", err)
	}
}

func TestTheAnswerPagesWithAnExactTotal(t *testing.T) {
	w := setupAccessWorld(t)
	limit := 2
	first, err := w.reads.Read(w.as(t, w.e.Rep1), "contact", w.workspaceContact, nil, &limit)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	all := w.everyone(t, w.e.Rep1, "contact", w.workspaceContact)
	if first.Page.Total == nil || *first.Page.Total != len(all) || len(first.Data) != limit || !first.Page.HasMore {
		t.Fatalf("first page: %d rows, total %v, has_more %v; want %d rows of %d", len(first.Data), first.Page.Total, first.Page.HasMore, limit, len(all))
	}
	counts := first.GroupCounts
	if counts.Owner+counts.Shared+counts.TeamShared+counts.Everyone != len(all) {
		t.Errorf("group counts %+v do not add up to %d members", counts, len(all))
	}
}

func hasCode(reasons []crmcontracts.RecordAccessReason, code crmcontracts.RecordAccessReasonCode) bool {
	return slices.ContainsFunc(reasons, func(r crmcontracts.RecordAccessReason) bool { return r.Code == code })
}

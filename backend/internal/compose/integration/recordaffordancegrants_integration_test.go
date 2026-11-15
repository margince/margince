// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// sharedRecords are records Rep3 owns and has shared with Rep1 for writing.
// Rep1's role is own-scoped, so the write share is Rep1's only row authority.
type sharedRecords struct {
	contact ids.ContactID
	company ids.CompanyID
	deal    ids.DealID
	lead    ids.LeadID
	project ids.ProjectID
}

func seedSharedRecords(t *testing.T, e *Env) sharedRecords {
	t.Helper()
	pipeline, open, _ := DealFixture(t, e)
	company := e.SeedCompany(t, "Shared company", &e.Rep3)
	out := sharedRecords{
		contact: ids.From[ids.ContactKind](e.SeedContact(t, "Shared contact", &e.Rep3)),
		company: ids.From[ids.CompanyKind](company),
		deal:    ids.From[ids.DealKind](e.SeedDeal(t, "Shared deal", pipeline, open, &e.Rep3)),
		lead:    seedLead(t, e, "Shared lead", "shared-lead@prospect.test", &e.Rep3),
		project: seedProject(e.Admin(), t, e, "Shared project", company, &e.Rep3).ID,
	}
	for _, record := range []struct {
		kind string
		id   ids.UUID
	}{
		{objContact, out.contact.UUID}, {objCompany, out.company.UUID}, {objDeal, out.deal.UUID},
		{objLead, out.lead.UUID}, {objProject, out.project.UUID},
	} {
		e.WsExec(t, `INSERT INTO record_grant (record_type, record_id, subject_type, subject_id, access, granted_by)
			VALUES ($1, $2, 'user', $3, 'write', $4)`, record.kind, record.id, e.Rep1, e.Rep3)
	}
	return out
}

// A write share makes a record editable but does not make its holder the
// owner. So the update grant alone offers no archive, and the delete grant
// offers it. Each flag must match what the mutation then does.
func TestRecordAffordancesOnASharedRecordFollowTheObjectGrants(t *testing.T) {
	e := Setup(t)
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	e.Contacts.SetClock(func() time.Time { return now })
	e.Deals.SetClock(func() time.Time { return now })
	shared := seedSharedRecords(t, e)
	for _, grants := range []recordWriteGrants{{}, {update: true}, {deleteGrant: true}, {update: true, deleteGrant: true}} {
		ctx := asTeam1Member(e, e.Rep1, recordAffordancePerms(grants))
		want := expectedRecordActions{writable: grants.update, archivable: grants.deleteGrant}
		contact, err := e.Contacts.GetContact(ctx, shared.contact, storekit.LiveOnly)
		if err != nil {
			t.Fatal(err)
		}
		requireRecordAffordances(t, contact.Writable, contact.Archivable, want)
		company, err := e.Contacts.GetCompany(ctx, shared.company, storekit.LiveOnly)
		if err != nil {
			t.Fatal(err)
		}
		requireRecordAffordances(t, company.Writable, company.Archivable, want)
		deal, err := e.Deals.GetDeal(ctx, shared.deal, storekit.LiveOnly)
		if err != nil {
			t.Fatal(err)
		}
		requireRecordAffordances(t, deal.Writable, deal.Archivable, want)
		lead, err := e.Contacts.GetLead(ctx, shared.lead, storekit.LiveOnly)
		if err != nil {
			t.Fatal(err)
		}
		requireWritable(t, lead.Writable, grants.update)
		project, err := e.Projects.GetProject(ctx, shared.project, storekit.LiveOnly)
		if err != nil {
			t.Fatal(err)
		}
		requireWritable(t, project.Writable, grants.update)
		if grants.update && !grants.deleteGrant {
			requireSharedArchiveRefused(ctx, t, e, shared)
		}
	}
	deleteOnly := asTeam1Member(e, e.Rep1, recordAffordancePerms(recordWriteGrants{deleteGrant: true}))
	if _, err := e.Deals.ArchiveDeal(deleteOnly, shared.deal, nil); err != nil {
		t.Fatalf("archive of a shared deal with the delete grant = %v, want allowed", err)
	}
	if _, err := e.Contacts.ArchiveContact(deleteOnly, shared.contact, nil); err != nil {
		t.Fatalf("archive of a shared contact with the delete grant = %v, want allowed", err)
	}
}

// requireSharedArchiveRefused archives as an update-only holder of a share:
// the flag said no, so the mutation must say no too.
func requireSharedArchiveRefused(ctx context.Context, t *testing.T, e *Env, shared sharedRecords) {
	t.Helper()
	if _, err := e.Deals.ArchiveDeal(ctx, shared.deal, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("update-only archive of a shared deal = %v, want permission denied", err)
	}
	if _, err := e.Contacts.ArchiveContact(ctx, shared.contact, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("update-only archive of a shared contact = %v, want permission denied", err)
	}
	if _, err := e.Contacts.ArchiveCompany(ctx, shared.company, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("update-only archive of a shared company = %v, want permission denied", err)
	}
}

// A read seat holding the rep's full grants is offered no action on any
// record, and every mutation it tries is refused.
func TestRecordAffordancesReadSeatIsOfferedNoActionAndRefusedEveryMutation(t *testing.T) {
	e := Setup(t)
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	e.Contacts.SetClock(func() time.Time { return now })
	e.Deals.SetClock(func() time.Time { return now })
	pipeline, open, _ := DealFixture(t, e)
	company := e.SeedCompany(t, "Seat company", &e.Rep1)
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Seat deal", pipeline, open, &e.Rep1))
	contact := ids.From[ids.ContactKind](e.SeedContact(t, "Seat contact", &e.Rep1))
	lead := seedLead(t, e, "Seat lead", "seat-lead@prospect.test", &e.Rep1)
	project := seedProject(e.Admin(), t, e, "Seat project", company, &e.Rep1).ID
	full := asTeam1Member(e, e.Rep1, recordAffordancePerms(recordWriteGrants{update: true, deleteGrant: true}))
	readSeat := OnSeat(full, principal.SeatRead)

	gotDeal, err := e.Deals.GetDeal(readSeat, deal, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	requireRecordAffordances(t, gotDeal.Writable, gotDeal.Archivable, expectedRecordActions{})
	gotContact, err := e.Contacts.GetContact(readSeat, contact, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	requireRecordAffordances(t, gotContact.Writable, gotContact.Archivable, expectedRecordActions{})
	gotLead, err := e.Contacts.GetLead(readSeat, lead, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	requireWritable(t, gotLead.Writable, false)
	gotProject, err := e.Projects.GetProject(readSeat, project, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	requireWritable(t, gotProject.Writable, false)

	name := "Renamed by a read seat"
	mutations := map[string]error{}
	_, mutations["deal update"] = e.Deals.UpdateDeal(readSeat, deal, deals.UpdateDealInput{Name: &name})
	_, mutations["deal archive"] = e.Deals.ArchiveDeal(readSeat, deal, nil)
	_, mutations["contact update"] = e.Contacts.UpdateContact(readSeat, contact, contacts.UpdateContactInput{FullName: &name})
	_, mutations["contact archive"] = e.Contacts.ArchiveContact(readSeat, contact, nil)
	_, mutations["lead update"] = e.Contacts.UpdateLead(readSeat, lead, contacts.UpdateLeadInput{FullName: &name})
	_, mutations["project update"] = e.Projects.UpdateProject(readSeat, project, projects.UpdateProjectInput{Name: &name})
	_, mutations["project archive"] = e.Projects.ArchiveProject(readSeat, project, nil)
	for what, err := range mutations {
		if !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("read seat %s = %v, want permission denied", what, err)
		}
	}
	// The same grants on a full seat may do it, so the refusal is the seat's.
	if _, err := e.Contacts.UpdateLead(full, lead, contacts.UpdateLeadInput{FullName: &name}); err != nil {
		t.Fatalf("full seat lead update = %v, want allowed", err)
	}
}

// An archived lead or project is nobody's to edit, while the owner keeps the
// update authority field masks read.
func TestArchivedLeadsAndProjectsOfferNoEdit(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Archive company", &e.Rep1)
	lead := seedLead(t, e, "Archived lead", "archived-lead@prospect.test", &e.Rep1)
	project := seedProject(e.Admin(), t, e, "Archived project", company, &e.Rep1).ID
	owner := asTeam1Member(e, e.Rep1, recordAffordancePerms(recordWriteGrants{update: true, deleteGrant: true}))
	live, err := e.Contacts.GetLead(owner, lead, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	requireWritable(t, live.Writable, true)
	e.WsExec(t, `UPDATE lead SET archived_at = now() WHERE id = $1`, lead)
	if _, err := e.Projects.ArchiveProject(owner, project, nil); err != nil {
		t.Fatal(err)
	}
	gotLead, err := e.Contacts.GetLead(owner, lead, storekit.IncludeArchived)
	if err != nil {
		t.Fatal(err)
	}
	requireWritable(t, gotLead.Writable, false)
	gotProject, err := e.Projects.GetProject(owner, project, storekit.IncludeArchived)
	if err != nil {
		t.Fatal(err)
	}
	requireWritable(t, gotProject.Writable, false)
	err = database.WithWorkspaceTx(owner, e.Pool, func(tx pgx.Tx) error {
		rows := []crmcontracts.Lead{gotLead}
		raw, err := auth.StampWritable(owner, tx, objLead, rows,
			func(l crmcontracts.Lead) ids.UUID { return ids.UUID(l.Id) },
			func(l *crmcontracts.Lead, may bool) { l.Writable = &may })
		if err != nil {
			return err
		}
		if !raw[lead.UUID] {
			t.Error("archiving the lead removed the owner's raw update authority")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// The stamps' own statement counts over a page of the caller's own record and
// a shared one. Reading row authority twice, or reading the owner rule where
// the delete grant already answers, changes them.
func TestRecordAffordanceStampsReadAuthorityOnce(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	shared := seedSharedRecords(t, e)
	own := e.SeedDeal(t, "Own deal", pipeline, open, &e.Rep1)
	ownLead := seedLead(t, e, "Own lead", "own-lead@prospect.test", &e.Rep1)
	cases := []struct {
		grants         recordWriteGrants
		deal, lead     int64
		ownArchivable  bool
		sharedArchives bool
	}{
		{grants: recordWriteGrants{}},
		// Update alone asks the owner rule once more, to tell the two rows apart.
		{grants: recordWriteGrants{update: true}, deal: 3, lead: 2, ownArchivable: true},
		{grants: recordWriteGrants{deleteGrant: true}, deal: 2, ownArchivable: true, sharedArchives: true},
		{grants: recordWriteGrants{update: true, deleteGrant: true}, deal: 2, lead: 2, ownArchivable: true, sharedArchives: true},
	}
	for _, tc := range cases {
		ctx := asTeam1Member(e, e.Rep1, recordAffordancePerms(tc.grants))
		err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			dealPage := []crmcontracts.Deal{{Id: openapi_types.UUID(own)}, {Id: openapi_types.UUID(shared.deal.UUID)}}
			metered, meter := database.WithStatementMeter(ctx)
			if _, err := auth.StampRecordAffordances(metered, tx, auth.DealArchive, dealPage,
				func(d crmcontracts.Deal) ids.UUID { return ids.UUID(d.Id) },
				func(d *crmcontracts.Deal, may bool) { d.Writable = &may },
				func(d *crmcontracts.Deal, may bool) { d.Archivable = &may }); err != nil {
				return err
			}
			if cost := meter.Cost(); cost.Statements != tc.deal || cost.Failed != 0 {
				t.Errorf("%+v: deal stamp cost = %+v, want %d statements", tc.grants, cost, tc.deal)
			}
			requireRecordAffordances(t, dealPage[0].Writable, dealPage[0].Archivable,
				expectedRecordActions{writable: tc.grants.update, archivable: tc.ownArchivable})
			requireRecordAffordances(t, dealPage[1].Writable, dealPage[1].Archivable,
				expectedRecordActions{writable: tc.grants.update, archivable: tc.sharedArchives})
			leadPage := []crmcontracts.Lead{{Id: openapi_types.UUID(ownLead.UUID)}, {Id: openapi_types.UUID(shared.lead.UUID)}}
			metered, meter = database.WithStatementMeter(ctx)
			if _, err := auth.StampWritable(metered, tx, objLead, leadPage,
				func(l crmcontracts.Lead) ids.UUID { return ids.UUID(l.Id) },
				func(l *crmcontracts.Lead, may bool) { l.Writable = &may }); err != nil {
				return err
			}
			if cost := meter.Cost(); cost.Statements != tc.lead || cost.Failed != 0 {
				t.Errorf("%+v: lead stamp cost = %+v, want %d statements", tc.grants, cost, tc.lead)
			}
			for _, row := range leadPage {
				requireWritable(t, row.Writable, tc.grants.update)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

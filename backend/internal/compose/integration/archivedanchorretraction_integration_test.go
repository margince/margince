// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An archived anchor freezes what its children GRANT, PUBLISH or ACCRUE, and
// never freezes what they REVOKE, VOID, CANCEL or RETRACT.
//
// Both halves are asserted together in every case here, on one archived record,
// because either half alone is satisfied by the wrong fix: a gate that refuses
// everything passes the freeze cases, and a gate that refuses nothing passes the
// retraction cases. What has to hold is that one archived anchor answers the two
// differently, and auth.EnsureWritableLive / auth.EnsureRetractable are the pair
// that says which side a call site took.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/commissions"
	"github.com/margince/margince/backend/internal/modules/dealrooms"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// retractionAdmin is the seat these cases act as. Spelled here rather than
// widened on a shared fixture, so a grant added for this suite cannot quietly
// widen another one.
var retractionAdmin = principal.Permissions{
	RoleKeys: []string{"admin"},
	Objects: map[string]principal.ObjectGrant{
		"person":       {Create: true, Read: true, Update: true, Delete: true},
		"organization": {Create: true, Read: true, Update: true, Delete: true},
		"deal":         {Create: true, Read: true, Update: true, Delete: true},
		"deal_room":    {Create: true, Read: true, Update: true, Delete: true},
	},
	RowScope: principal.RowScopeAll,
}

// The decisive case for the whole rule. A deal room is buyer-facing access that
// survives precisely because the deal it hangs off was retired, so archiving the
// deal is the moment somebody reaches to cut a buyer off — and the moment a
// blanket "the anchor blocks" would take that control away.
func TestArchivingADealFreezesItsRoomsInvitesAndNeverItsRevocations(t *testing.T) {
	e := Setup(t)
	ctx := e.As(e.AdminUser, nil, retractionAdmin)
	rooms := dealrooms.NewStore(e.DB())

	pipeline, open, _ := DealFixture(t, e)
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Northgate rollout", pipeline, open, &e.Rep1))
	room, err := rooms.CreateRoom(ctx, dealrooms.CreateRoomInput{
		DealID: deal, Title: "Northgate rollout", Source: "ui",
	})
	if err != nil {
		t.Fatalf("opening the room: %v", err)
	}
	roomID := ids.From[ids.DealRoomKind](ids.UUID(room.Id))
	invited, err := rooms.InviteParticipant(ctx, roomID, dealrooms.InviteInput{
		FullName: "Laura Buyer", Email: "laura@buyer.example", Capability: "comment", Source: "ui",
	})
	if err != nil {
		t.Fatalf("seating the buyer: %v", err)
	}
	seat := ids.From[ids.DealRoomParticipantKind](ids.UUID(invited.Participant.Id))

	// Retired through the product's own writer, so the rows sit in the state the
	// product leaves them in rather than one this test invented.
	if _, err := e.Deals.ArchiveDeal(ctx, deal, nil); err != nil {
		t.Fatalf("archiving the deal: %v", err)
	}

	// Frozen: every way of handing MORE access out.
	if _, err := rooms.InviteParticipant(ctx, roomID, dealrooms.InviteInput{
		FullName: "Second Buyer", Email: "second@buyer.example", Capability: "comment", Source: "ui",
	}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("inviting onto an archived deal: got %v, want not found", err)
	}
	if _, err := rooms.ResendInvitation(ctx, roomID, seat); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("reissuing a credential on an archived deal: got %v, want not found", err)
	}
	if _, err := rooms.PreviewRoom(ctx, roomID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("minting a preview seat on an archived deal: got %v, want not found", err)
	}

	// Never frozen: taking access away. Pause first — resume is the grant half
	// of the same lifecycle and must refuse from the state pause leaves.
	if _, err := rooms.PauseRoom(ctx, roomID); err != nil {
		t.Fatalf("pausing a room whose deal was archived: %v", err)
	}
	if _, err := rooms.ResumeRoom(ctx, roomID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("resuming a room whose deal was archived: got %v, want not found", err)
	}
	revoked, err := rooms.RevokeParticipant(ctx, roomID, seat)
	if err != nil {
		t.Fatalf("revoking a seat whose deal was archived: %v", err)
	}
	if revoked.RevokedAt == nil {
		t.Error("the seat came back unrevoked")
	}
	if _, err := rooms.ArchiveRoom(ctx, roomID, nil); err != nil {
		t.Fatalf("ending a room whose deal was archived: %v", err)
	}
}

// A file on a retired deal must still be delistable, and must not be put back:
// the two branches of one gate, on one archived anchor.
func TestArchivingADealFreezesRelistingItsDocumentAndNeverDelisting(t *testing.T) {
	e := Setup(t)
	ctx := e.As(e.AdminUser, nil, retractionAdmin)
	files, _ := attachmentStore(e)

	pipeline, open, _ := DealFixture(t, e)
	deal := e.SeedDeal(t, "Northgate rollout", pipeline, open, &e.Rep1)
	filed, err := files.UploadAttachment(ctx, activities.AttachmentInput{
		EntityType: "deal", EntityID: deal,
		Filename: "msa.pdf", ContentType: "application/pdf",
		Content: strings.NewReader("PDF-BYTES"),
	})
	if err != nil {
		t.Fatalf("filing the document: %v", err)
	}
	attachment := ids.UUID(filed.Id)

	if _, err := e.Deals.ArchiveDeal(ctx, ids.From[ids.DealKind](deal), nil); err != nil {
		t.Fatalf("archiving the deal: %v", err)
	}

	if err := files.HideDealDocument(ctx, deal, attachment); err != nil {
		t.Fatalf("delisting a document on an archived deal: %v", err)
	}
	if err := files.UnhideDealDocument(ctx, deal, attachment); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("re-listing a document on an archived deal: got %v, want not found", err)
	}
	var hidden int
	if err := e.DB().Tx(context.Background(), func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM deal_document_hide WHERE deal_id = $1 AND attachment_id = $2`,
			deal, attachment).Scan(&hidden)
	}); err != nil {
		t.Fatalf("reading the hide back: %v", err)
	}
	if hidden != 1 {
		t.Errorf("deal_document_hide rows = %d, want the delisting to stand", hidden)
	}
}

// The same asymmetry on money: an accrual on a deal that has since been retired
// is exactly the one somebody needs to take back, and none of it may be
// approved or paid out any more.
func TestArchivingADealFreezesACommissionApprovalAndNeverItsVoid(t *testing.T) {
	e := Setup(t)
	fx := seedAccrualFixture(t, e, "tier2_20")
	admin := e.As(e.AdminUser, nil, commissionAdminPerms)

	winAndDeliver(t, e, fx)
	entries, err := fx.ledger.List(admin, commissions.ListInput{DealID: &fx.deal})
	if err != nil || len(entries.Data) != 1 {
		t.Fatalf("ledger after the win: %v %+v", err, entries.Data)
	}
	entry := ids.From[ids.CommissionEntryKind](ids.UUID(entries.Data[0].Id))

	if _, err := e.Deals.ArchiveDeal(admin, fx.deal, nil); err != nil {
		t.Fatalf("archiving the won deal: %v", err)
	}

	if _, err := fx.ledger.Decide(admin, entry, commissions.DecideInput{
		Decision: commissions.DecisionApprove,
	}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("approving an accrual on an archived deal: got %v, want not found", err)
	}
	reason := "the deal was retired"
	voided, err := fx.ledger.Decide(admin, entry, commissions.DecideInput{
		Decision: commissions.DecisionVoid, Reason: &reason,
	})
	if err != nil {
		t.Fatalf("voiding an accrual on an archived deal: %v", err)
	}
	if string(voided.Status) != commissions.StatusVoid {
		t.Errorf("status after the void = %q, want void", voided.Status)
	}
	// Read off the table rather than through the ledger's own list: that list
	// composes VisibleClause, which refuses an archived deal's entries by
	// design, so it would answer "nothing live" whether or not the void landed.
	var live, reversals int
	if err := e.DB().Tx(context.Background(), func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FILTER (WHERE status <> 'void'),
			        count(*) FILTER (WHERE reversal_of IS NOT NULL)
			   FROM commission_entry WHERE deal_id = $1`, fx.deal).Scan(&live, &reversals)
	}); err != nil {
		t.Fatalf("reading the ledger back: %v", err)
	}
	if live != 0 || reversals != 1 {
		t.Errorf("after the void: %d live entries and %d reversals, want 0 and 1", live, reversals)
	}
}

// A share standing on a record that has since been archived is stale state, and
// retiring the record is precisely when somebody reaches to clear it. Before
// this it could not be cleared at all: mayRevoke probed the live spelling.
func TestAShareOnAnArchivedRecordCanStillBeRevoked(t *testing.T) {
	e := Setup(t)
	ctx := e.As(e.AdminUser, nil, retractionAdmin)
	org := e.SeedOrg(t, "Northgate Holding", &e.Rep1)

	shares := identity.NewServiceFor(e.DB())
	grant, err := shares.CreateRecordGrant(ctx, identity.CreateGrantInput{
		RecordType: "organization", RecordID: org,
		SubjectType: "user", SubjectID: e.Rep3, Access: "read",
	})
	if err != nil {
		t.Fatalf("sharing the record: %v", err)
	}

	if _, err := e.People.ArchiveOrganization(ctx, ids.From[ids.OrganizationKind](org), nil); err != nil {
		t.Fatalf("archiving the shared record: %v", err)
	}

	// Frozen: a NEW share of a retired record is a claim, and refused.
	if _, err := shares.CreateRecordGrant(ctx, identity.CreateGrantInput{
		RecordType: "organization", RecordID: org,
		SubjectType: "user", SubjectID: e.Rep2, Access: "read",
	}); err == nil {
		t.Error("sharing an archived record succeeded")
	}
	// Never frozen: clearing the share that is already standing.
	if err := shares.RevokeRecordGrant(ctx, grant.ID); err != nil {
		t.Fatalf("revoking a share on an archived record: %v", err)
	}
	var left int
	if err := e.DB().Tx(context.Background(), func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM record_grant WHERE record_id = $1`, org).Scan(&left)
	}); err != nil {
		t.Fatalf("counting the shares left: %v", err)
	}
	if left != 0 {
		t.Errorf("record_grant rows = %d after the revoke, want 0", left)
	}
}

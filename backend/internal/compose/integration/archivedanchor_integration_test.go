// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Archived means frozen, and it reaches a row that carries no authority of its
// own through the record it hangs off.
//
// Every suite here runs UNBOUNDED on purpose. auth.EnsureVisible returns nil
// without so much as an existence check for an actor that reads every row of
// the table, so an unbounded seat is the one these gates failed open for — a
// bounded rep is refused by the scope clause and never reaches the liveness
// question at all. Running them as a rep would report green over the defects.
//
// The exception is worth knowing before reading the contract suite: person and
// organization are ownerPrivateTables, so no non-system principal is ever
// unbounded on them. That is why the contract case behaves differently from the
// attachment and deal-room ones, and its own comment says so.

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/modules/activities"
	"github.com/gradionhq/margince/backend/internal/modules/contracts"
	"github.com/gradionhq/margince/backend/internal/modules/dealrooms"
	"github.com/gradionhq/margince/backend/internal/modules/deals"
	"github.com/gradionhq/margince/backend/internal/platform/blobstore"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
)

// A file uploaded onto an archived parent could never be fetched again: the
// read path (resolveVisibleAttachmentParent) requires a live parent and the
// upload gate did not, so the bytes reached the object store and the row
// reached the table with nothing in the product able to read either. The
// activity arm always refused; the deal/person/organization arm did not, which
// is the whole reason both live in one function.
func TestUploadingOntoAnArchivedParentIsRefused(t *testing.T) {
	e := Setup(t)
	ctx := e.Admin()
	store := activities.NewStore(e.DB()).WithBlobstore(blobstore.NewMemory())
	pipeline, open, _ := DealFixture(t, e)

	person := e.SeedPerson(t, "Archived Parent", &e.Rep1)
	org := e.SeedOrg(t, "Archived Parent GmbH", &e.Rep1)
	deal := e.SeedDeal(t, "Archived Parent deal", pipeline, open, &e.Rep1)

	for _, tc := range []struct {
		entityType string
		id         ids.UUID
	}{
		{"person", person},
		{"organization", org},
		{"deal", deal},
	} {
		e.WsExec(t, `UPDATE `+tc.entityType+` SET archived_at = now() WHERE id = $1`, tc.id)
		t.Run(tc.entityType, func(t *testing.T) {
			_, err := store.UploadAttachment(ctx, activities.AttachmentInput{
				EntityType: tc.entityType, EntityID: tc.id,
				Filename: "offer.pdf", ContentType: "application/pdf",
				Content: strings.NewReader("PDF-BYTES"),
			})
			if !errors.Is(err, apperrors.ErrNotFound) {
				t.Fatalf("upload onto an archived %s: got %v, want not found", tc.entityType, err)
			}
		})
	}

	// No row, and so no orphan object either: the gate runs before any bytes are
	// written, which is the property the upload's own docblock claims for it.
	var rows int
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT count(*) FROM attachment`).Scan(&rows)
	}); err != nil {
		t.Fatalf("count attachments: %v", err)
	}
	if rows != 0 {
		t.Errorf("attachment rows = %d, want 0 — an upload landed on an archived parent", rows)
	}
}

// A contract carries no owner of its own, so its anchor being archived freezes
// it — for a human caller this already held, and the point of pinning it is
// that it now holds at BOTH layers rather than at one.
//
// Where it held: readContract composes contracts.VisibleClause, whose deal and
// organization arms each carry archived_at IS NULL. That clause is skipped only
// when the caller reads every row of both anchors with no predicate at all —
// and organization is an ownerPrivateTable, so auth.UnboundedFor answers false
// for every principal except the SYSTEM one. A human, admin included, was
// refused by the read and never reached the write probe.
//
// Where it did not: the system principal, and the write probe itself, which
// asked EnsureWritable. That is the residue #1398 recorded as "only reaches a
// caller with unbounded row scope" and deferred as failing closed. Ruling 4 on
// #2145 settles it the strict way, so the write agrees with the read instead of
// depending on it — which is what the note about a second hand-composed clause
// was asking for.
//
// So this suite is a specification of the invariant, not a regression test for
// the probe swap: it passes before and after, and it is here because nothing
// else pinned the anchor rule for contracts end to end.
func TestChangingAContractWhoseAnchorIsArchivedIsRefused(t *testing.T) {
	e := Setup(t)
	ctx := e.Admin()
	pipeline, open, _ := DealFixture(t, e)

	for _, tc := range []struct {
		name   string
		anchor string
		seed   func() (contracts.CreateContractInput, ids.UUID)
	}{
		{
			name:   "an organization-anchored contract",
			anchor: "organization",
			seed: func() (contracts.CreateContractInput, ids.UUID) {
				org := e.SeedOrg(t, "Anchor GmbH", &e.Rep1)
				return contracts.CreateContractInput{
					OrganizationID: ids.From[ids.OrganizationKind](org),
					Title:          "MSA", ValueBasis: contracts.BasisTotal, Source: "manual",
				}, org
			},
		},
		{
			name:   "a deal-anchored contract",
			anchor: "deal",
			seed: func() (contracts.CreateContractInput, ids.UUID) {
				org := e.SeedOrg(t, "Anchor Two GmbH", &e.Rep1)
				orgID := ids.From[ids.OrganizationKind](org)
				owner := ids.From[ids.UserKind](e.Rep1)
				// Created here rather than through SeedDeal: a deal-anchored
				// contract reads its deal's organization, and the shared helper
				// leaves that column NULL.
				d, err := e.Deals.CreateDeal(ctx, deals.CreateDealInput{
					Name: "Anchor deal", PipelineID: pipeline, StageID: open,
					OrganizationID: &orgID, OwnerID: &owner,
				})
				if err != nil {
					t.Fatalf("seed deal: %v", err)
				}
				dealID := ids.From[ids.DealKind](ids.UUID(d.Id))
				return contracts.CreateContractInput{
					OrganizationID: orgID,
					DealID:         &dealID,
					Title:          "SOW", ValueBasis: contracts.BasisTotal, Source: "manual",
				}, ids.UUID(d.Id)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in, anchorID := tc.seed()
			created, err := e.Contracts.CreateContract(ctx, in)
			if err != nil {
				t.Fatalf("seed contract: %v", err)
			}
			id := ids.From[ids.ContractKind](ids.UUID(created.Id))
			e.WsExec(t, `UPDATE `+tc.anchor+` SET archived_at = now() WHERE id = $1`, anchorID)

			title := "Renamed after the anchor was retired"
			if _, err := e.Contracts.UpdateContract(ctx, id,
				crmcontracts.UpdateContractRequest{Title: &title}, nil); !errors.Is(err, apperrors.ErrNotFound) {
				t.Fatalf("patching a contract whose %s is archived: got %v, want not found", tc.anchor, err)
			}
			if err := e.Contracts.ArchiveContract(ctx, id); !errors.Is(err, apperrors.ErrNotFound) {
				t.Errorf("archiving a contract whose %s is archived: got %v, want not found", tc.anchor, err)
			}
		})
	}
}

// roomAnchorAdmin is the seat the room suite acts as: unbounded, holding the
// deal_room grants its writes need. Spelled here rather than widened on the
// shared admin fixture, so a grant added for this suite cannot quietly widen
// every other one.
//
// Unbounded is the point. `deal` is not an ownerPrivateTable, so
// auth.UnboundedFor answers TRUE for a RowScopeAll seat and auth.EnsureVisible
// then returns nil without checking anything at all — which is exactly the
// caller the room's write gate used to admit against a retired deal.
var roomAnchorAdmin = principal.Permissions{
	RoleKeys: []string{"admin"},
	Objects: map[string]principal.ObjectGrant{
		"person":    {Create: true, Read: true, Update: true, Delete: true},
		"deal":      {Create: true, Read: true, Update: true, Delete: true},
		"deal_room": {Create: true, Read: true, Update: true, Delete: true},
		"activity":  {Create: true, Read: true, Update: true, Delete: true},
	},
	RowScope: principal.RowScopeAll,
}

// A deal room is its deal's child and holds no authority of its own, so
// archiving the deal freezes the room with it. Before this the room stayed
// editable: a buyer could be invited into — and content published from — a room
// whose deal the product had already retired.
func TestWritingToADealRoomWhoseDealIsArchivedIsRefused(t *testing.T) {
	e := Setup(t)
	ctx := e.As(e.AdminUser, nil, roomAnchorAdmin)
	rooms := dealrooms.NewStore(e.DB())

	dealID := e.SeedWonDealLinkedTo(t)
	room, err := rooms.CreateRoom(ctx, dealrooms.CreateRoomInput{
		DealID: ids.From[ids.DealKind](dealID), Title: "Anchor room", Source: "ui",
	})
	if err != nil {
		t.Fatalf("seed the room while its deal is live: %v", err)
	}
	roomID := ids.From[ids.DealRoomKind](ids.UUID(room.Id))

	e.WsExec(t, `UPDATE deal SET archived_at = now() WHERE id = $1`, dealID)

	if _, err := rooms.InviteParticipant(ctx, roomID, dealrooms.InviteInput{
		FullName: "Rita Reviewer", Email: "rita@anchor.test",
		Capability: "comment", Source: "ui",
	}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("inviting into a room whose deal is archived: got %v, want not found", err)
	}

	var seats int
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM deal_room_participant WHERE room_id = $1`, roomID).Scan(&seats)
	}); err != nil {
		t.Fatalf("count participants: %v", err)
	}
	if seats != 0 {
		t.Errorf("deal_room_participant rows = %d, want 0 — a seat was minted in a retired deal's room", seats)
	}
}

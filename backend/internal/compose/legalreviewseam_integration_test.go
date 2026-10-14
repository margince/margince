// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The review pack's documents through the real Files-area reader. A contract
// mailed against the deal is packed for the seat that can read the mail. A
// file of another category is not. A colleague outside the message's audience
// neither packs it nor sees it in a pack as a reviewer.

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func TestTheReviewPackCarriesTheContractsTheRequesterCanRead(t *testing.T) {
	ctx, db, tag := captureWorkspace(t)
	sink := capture.NewSink(db).WithFileKeeper(fileKeeper(db.Pool(), blobstore.NewMemory()))
	dealID := seedDealForFiles(ctx, t, db)
	rec := withFiles(mailRecord("msg-review-"+tag), onePDF())
	rec.Links = []datasource.EntityRef{{Type: datasource.EntityDeal, ID: dealID}}
	if _, err := sink.Upsert(ctx, rec); err != nil {
		t.Fatalf("capture: %v", err)
	}
	store := activities.NewStore(db)
	seam := legalReviewDocuments{store: store}
	rep := asRep(ctx, captureSeat(ctx))

	packed, err := seam.ReviewDocuments(rep, dealID)
	if err != nil {
		t.Fatalf("packing: %v", err)
	}
	if len(packed) != 0 {
		t.Fatalf("a file still filed as a mail attachment was packed: %+v", packed)
	}

	listed, _, err := store.ListDealDocuments(rep, dealID, activities.DealDocumentFilters{})
	if err != nil || len(listed) != 1 {
		t.Fatalf("the deal lists %d files (%v), want the mailed PDF", len(listed), err)
	}
	contract, draft := "contract", "draft"
	editor := principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + captureSeat(ctx).String(), UserID: captureSeat(ctx),
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"}, RowScope: principal.RowScopeAll,
			Objects: map[string]principal.ObjectGrant{"activity": {Read: true, Update: true}, "deal": {Read: true, Update: true}},
		},
	})
	attachmentID := ids.UUID(listed[0].Attachment.Id)
	if _, err := store.UpdateAttachmentMetadata(editor, attachmentID, activities.DocumentMetadata{
		Category: &contract, DocState: &draft,
	}); err != nil {
		t.Fatalf("filing the PDF as a contract: %v", err)
	}

	packed, err = seam.ReviewDocuments(rep, dealID)
	if err != nil {
		t.Fatalf("packing: %v", err)
	}
	if len(packed) != 1 || packed[0].AttachmentID != attachmentID || packed[0].DocState != draft || !packed[0].FromMessage {
		t.Fatalf("the pack carried %+v, want the draft contract", packed)
	}

	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE activity SET audience = 'participants' WHERE source_system = 'email' AND source_id = $1`, "msg-review-"+tag)
		return err
	}); err != nil {
		t.Fatalf("narrow the audience: %v", err)
	}
	outsider, err := seam.ReviewDocuments(asRep(ctx, ids.NewV7()), dealID)
	if err != nil {
		t.Fatalf("packing for a colleague: %v", err)
	}
	if len(outsider) != 0 {
		t.Fatalf("a colleague outside the message's audience packed %+v", outsider)
	}

	// The reviewer side: the mailbox owner opens the file, a reviewer outside
	// the message's audience is not told it exists.
	opened, err := seam.OpenableDocuments(rep, []ids.UUID{attachmentID})
	if err != nil || opened[attachmentID].Filename != "contract.pdf" {
		t.Fatalf("the mailbox owner opened %+v (%v), want contract.pdf", opened, err)
	}
	reviewer, err := seam.OpenableDocuments(asRep(ctx, ids.NewV7()), []ids.UUID{attachmentID})
	if err != nil {
		t.Fatalf("asking for a reviewer outside the audience: %v", err)
	}
	if len(reviewer) != 0 {
		t.Fatalf("a reviewer outside the message's audience was shown %+v", reviewer)
	}
}

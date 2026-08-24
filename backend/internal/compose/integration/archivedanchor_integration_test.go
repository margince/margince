// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Archived means frozen, and it reaches a row that carries no authority of its
// own through the record it hangs off.
//
// This runs as ADMIN on purpose. auth.EnsureVisible returns nil without so much
// as an existence check for an actor that reads every row of the table, so an
// unbounded seat is the one this gate failed open for; a bounded rep is refused
// by the scope clause and never reaches the liveness question at all. Run as a
// rep, this suite would report green over a live defect.

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/modules/activities"
	"github.com/gradionhq/margince/backend/internal/platform/blobstore"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
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

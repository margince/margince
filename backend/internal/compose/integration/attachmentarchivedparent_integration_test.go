// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/modules/activities"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// Hanging a file on a record changes that record, so an archived one refuses
// it. Before this the upload wrote an attachment row, an audit row and an
// outbox event against a deal, person or organization that every live write
// path already refused. The activity arm always refused — EnsureActivityWritable
// reaches EnsureActivityContentVisibleLive — and the deal/person/organization
// arm did not, which is the whole reason both live in one function.
//
// This says nothing about whether such a file can be READ back. It can:
// ensureAttachmentParentVisible (activities/attachment.go) uses the non-live
// probe, so a download or a metadata read of an archived parent's attachment
// still succeeds. Whether a read owes the same liveness a write does is a
// separate question and is not settled here.
func TestUploadingOntoAnArchivedParentIsRefused(t *testing.T) {
	e := Setup(t)
	ctx := e.Admin()
	store, _ := attachmentStore(e)
	pipeline, open, _ := DealFixture(t, e)

	person := e.SeedPerson(t, "Archived Parent", &e.Rep1)
	org := e.SeedOrg(t, "Archived Parent GmbH", &e.Rep1)
	deal := e.SeedDeal(t, "Archived Parent deal", pipeline, open, &e.Rep1)

	// Retired through the product's own archive writers rather than by column,
	// so the rows sit in the state the product actually leaves them in —
	// cascades, audit rows and all — instead of one this test invented.
	if _, err := e.People.ArchivePerson(ctx, ids.From[ids.PersonKind](person), nil); err != nil {
		t.Fatalf("archive person: %v", err)
	}
	if _, err := e.People.ArchiveOrganization(ctx, ids.From[ids.OrganizationKind](org), nil); err != nil {
		t.Fatalf("archive organization: %v", err)
	}
	if _, err := e.Deals.ArchiveDeal(ctx, ids.From[ids.DealKind](deal), nil); err != nil {
		t.Fatalf("archive deal: %v", err)
	}

	for _, tc := range []struct {
		entityType string
		id         ids.UUID
	}{
		{"person", person},
		{"organization", org},
		{"deal", deal},
	} {
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

	// Asserted on the table rather than inferred from the three refusals: a
	// gate that returned the right error while still committing a row is the
	// failure this is here to notice. It does NOT speak for the object store —
	// UploadAttachment's docblock is explicit that a failure after the put
	// leaves an orphan object by design, so the row is the durable claim and
	// the only one worth pinning.
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

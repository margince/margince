// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A file a private message carried is recorded by name only. Every reader that
// would reach for its bytes stops at the row, and no library lists it.

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// withheldFileOnDealMail logs a mail filed on a deal and records one withheld
// file on it through the real writer, returning the deal and the file.
func withheldFileOnDealMail(t *testing.T, e *Env, store *activities.Store) (ids.UUID, ids.UUID) {
	t.Helper()
	pipeline, open, _ := DealFixture(t, e)
	deal := e.SeedDeal(t, "Withheld File Deal", pipeline, open, &e.Rep1)
	subject, direction, at := "Private", "inbound", time.Now()
	mail, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Direction: &direction, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal}},
	})
	if err != nil {
		t.Fatalf("logging the mail: %v", err)
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return store.RecordWithheldFiles(e.Admin(), tx, ids.From[ids.ActivityKind](ids.UUID(mail.Id)),
			activities.CapturedFileSource{
				System: "imap", MessageID: "m-withheld", CapturedBy: "connector:imap", Category: "email_attachment",
			}, []activities.WithheldFile{{
				PartID: "part:1", Filename: "payslip.pdf", ContentType: "application/pdf", ByteSize: 4096,
			}})
	}); err != nil {
		t.Fatalf("recording the withheld file: %v", err)
	}
	var file ids.UUID
	if err := e.Pool.QueryRow(e.Admin(),
		`SELECT id FROM attachment WHERE activity_id = $1`, mail.Id).Scan(&file); err != nil {
		t.Fatalf("reading back the withheld file: %v", err)
	}
	return deal, file
}

func TestAWithheldFileHasNothingToDownloadOrRead(t *testing.T) {
	e := Setup(t)
	store, _ := attachmentStore(e)
	_, file := withheldFileOnDealMail(t, e, store)

	meta, rc, err := store.OpenAttachment(e.Admin(), file)
	if rc != nil {
		t.Error("a reader was opened for a file whose bytes were never kept")
	}
	if !errors.Is(err, activities.ErrBytesWithheld) || !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("OpenAttachment = %+v, %v; want ErrBytesWithheld answering as not found", meta, err)
	}

	if _, _, err := store.StartExtractionReadQueued(e.Admin(), file, "human:"+e.AdminUser.String(), nil); !errors.Is(err, activities.ErrBytesWithheld) {
		t.Fatalf("StartExtractionReadQueued err = %v, want ErrBytesWithheld — a reading of no bytes reads nothing", err)
	}
}

func TestAWithheldFileStaysOutOfTheDealsFiles(t *testing.T) {
	e := Setup(t)
	store, _ := attachmentStore(e)
	deal, file := withheldFileOnDealMail(t, e, store)

	docs, _, err := store.ListDealDocuments(e.Admin(), deal, activities.DealDocumentFilters{})
	if err != nil {
		t.Fatalf("ListDealDocuments: %v", err)
	}
	for _, doc := range docs {
		if ids.UUID(doc.Attachment.Id) == file {
			t.Fatal("the deal's Files area lists a file nobody can open")
		}
	}
}

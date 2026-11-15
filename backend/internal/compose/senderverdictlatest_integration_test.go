// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"bytes"
	"context"
	"encoding/base64"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestANewerBusinessVerdictCancelsPersonalDestruction(t *testing.T) {
	for _, kind := range []string{capture.KindCompanySender, capture.KindRoleMailbox} {
		t.Run(kind, func(t *testing.T) {
			e := integration.Setup(t)
			const sender = "office@correspondent.example"
			mail := seedPurgeableMail(t, e, sender, "Business reply", e.Rep1)
			resolvePersonal(t, e, sender, mail, e.Rep1, false)
			agePersonalMail(t, e, mail, sender, 31*24*time.Hour)
			resolveSenderAs(t, e, sender, mail, e.Rep1, false, capture.PendingStatusReal, kind)

			list, err := capture.SendersFor(purgeCtx(e, e.Rep1), InstallationDB(e.Pool), capture.DefaultPersonalPurgeWindows())
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, row := range list {
				if row.Address == sender {
					found = true
					if row.Kind != kind || !row.DeletesAt.IsZero() {
						t.Fatalf("sender = %+v, want business with no deletion date", row)
					}
				}
			}
			if !found {
				t.Fatal("the current sender disappeared")
			}
			if n := sweepPersonal(t, e); n != 0 {
				t.Fatalf("destroyed %d messages after a newer business verdict", n)
			}
			if activityBody(t, e, mail) == "" {
				t.Fatal("the business message lost its body")
			}
			if importCount(t, e, mail, e.Rep1) != 1 {
				t.Fatal("the seat lost its claim on the business message")
			}
		})
	}
}

func TestANewerBusinessVerdictKeepsAttachmentBytes(t *testing.T) {
	e := integration.Setup(t)
	ctx := asConnectorFor(e.Admin(), "connector:imap", e.Rep1)
	db, tag := InstallationDB(e.Pool), e.WS.String()
	sink := newCaptureSink(db.Pool(), CaptureConfig{}).WithFileKeeper(fileKeeper(db.Pool(), blobstore.NewMemory()))
	address := "readmitted-" + tag + "@example.com"
	first := "msg-first-" + tag
	if _, err := sink.Upsert(ctx, fromAddress(mailRecord(first), address)); err != nil {
		t.Fatal(err)
	}
	resolveCapturedSenderAs(ctx, t, db, address, capture.PendingStatusNoise, capture.KindPersonal)
	if err := db.Tx(ctx, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE capture_pending_counterparty SET resolved_at = now() - interval '15 days' WHERE owner_id = @seat AND email = @address AND resolved_at IS NOT NULL`, pgx.NamedArgs{"seat": e.Rep1, "address": address})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := sink.Upsert(ctx, fromAddress(mailRecord("msg-rejudge-"+tag), address)); err != nil {
		t.Fatal(err)
	}
	resolveCapturedSenderAs(ctx, t, db, address, capture.PendingStatusReal, capture.KindCompanySender)
	source := "msg-business-" + tag
	rec := withAttachedOriginal(withFiles(fromAddress(mailRecord(source), address), onePDF()))
	if _, err := sink.Upsert(ctx, rec); err != nil {
		t.Fatal(err)
	}
	files := filesFor(ctx, t, db, source)
	if len(files) != 1 || files[0].withheld || files[0].storageKey == "" {
		t.Fatalf("business attachment = %+v, want stored bytes", files)
	}
	encoded := base64.StdEncoding.EncodeToString(onePDF().Body)
	if !bytes.Contains(storedOriginal(ctx, t, db, source), []byte(encoded)) {
		t.Fatal("the business original lost its attachment bytes")
	}
}

func TestANewerVerdictOfAnotherSeatDoesNotCancelPersonalDestruction(t *testing.T) {
	e := integration.Setup(t)
	const sender = "shared-address@example.test"
	mail := seedPurgeableMail(t, e, sender, "Private correspondence", e.Rep1)
	resolvePersonal(t, e, sender, mail, e.Rep1, false)
	agePersonalMail(t, e, mail, sender, 31*24*time.Hour)
	resolveSenderAs(t, e, sender, mail, e.Rep2, false, capture.PendingStatusReal, capture.KindCompanySender)
	if n := sweepPersonal(t, e); n != 1 {
		t.Fatalf("destroyed %d messages, want the first seat's personal mail destroyed", n)
	}
}

func TestANewerBusinessVerdictInvalidatesAnAlreadySelectedPersonalPurge(t *testing.T) {
	e := integration.Setup(t)
	const sender = "rejudged@example.test"
	mail := seedPurgeableMail(t, e, sender, "Business correspondence", e.Rep1)
	resolvePersonal(t, e, sender, mail, e.Rep1, false)
	agePersonalMail(t, e, mail, sender, 31*24*time.Hour)
	var subject capture.PurgeSubject
	windows := capture.DefaultPersonalPurgeWindows()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		floor, _, err := purgeFloorTx(e.Admin(), tx)
		if err != nil {
			return err
		}
		subject, err = capture.SelectPersonalPurgeTx(e.Admin(), tx, e.Rep1, windows, floor, 10)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(subject.SoleImports) != 1 || subject.SoleImports[0] != mail {
		t.Fatalf("selected %+v, want the personal message", subject)
	}
	resolveSenderAs(t, e, sender, mail, e.Rep1, false, capture.PendingStatusReal, capture.KindCompanySender)
	purger := NewCapturePurger(e.Pool, NewRetentionServiceFor(InstallationDB(e.Pool), nil, slog.Default()))
	n, err := purger.purgePersonalSelection(principal.SystemActing(e.Admin(), personalSweepActor), subject, e.Rep1, windows)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 || activityBody(t, e, mail) == "" {
		t.Fatal("a stale personal selection destroyed newer business correspondence")
	}
	if importCount(t, e, mail, e.Rep1) != 1 {
		t.Fatal("a stale selection released the business correspondence")
	}
}

func TestALatePersonalResolutionCannotReplaceANewerBusinessQuestion(t *testing.T) {
	e := integration.Setup(t)
	const sender = "late-question@example.test"
	mail := seedPurgeableMail(t, e, sender, "Business correspondence", e.Rep1)
	resolvePersonal(t, e, sender, mail, e.Rep1, false)
	resolveSenderAs(t, e, sender, mail, e.Rep1, false, capture.PendingStatusReal, capture.KindCompanySender)
	ageOneActivity(t, e, mail, 34*24*time.Hour)
	e.WsExec(t, `UPDATE capture_pending_counterparty
  SET created_at = now() - CASE WHEN status = 'noise' THEN interval '34 days' ELSE interval '33 days' END,
      resolved_at = now() - CASE WHEN status = 'noise' THEN interval '31 days' ELSE interval '32 days' END
  WHERE owner_id = @owner AND email = @email`, pgx.NamedArgs{"owner": e.Rep1, "email": sender})
	if n := sweepPersonal(t, e); n != 0 {
		t.Fatalf("destroyed %d messages on a late answer to an older question", n)
	}
	if activityBody(t, e, mail) == "" {
		t.Fatal("the obsolete personal answer destroyed business correspondence")
	}
	list, err := capture.SendersFor(purgeCtx(e, e.Rep1), InstallationDB(e.Pool), capture.DefaultPersonalPurgeWindows())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range list {
		if row.Address == sender {
			found = true
			if row.Kind != capture.KindCompanySender || !row.DeletesAt.IsZero() {
				t.Fatalf("sender = %+v, want business with no deletion date", row)
			}
		}
	}
	if !found {
		t.Fatal("the current business sender disappeared")
	}
}

func resolveCapturedSenderAs(ctx context.Context, t *testing.T, db *database.DB, address, status, kind string) {
	t.Helper()
	pending := capture.NewPendingStore(db)
	due, err := pending.ClaimDue(ctx, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range due {
		if row.Email != address {
			continue
		}
		var won bool
		if err := db.Tx(ctx, func(tx pgx.Tx) error {
			var err error
			won, err = pending.ResolveAs(ctx, tx, row, status, kind, "test", false, capture.VerdictMeasurement{})
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if !won {
			t.Fatal("the sender verdict lost its own claim")
		}
		return
	}
	t.Fatalf("the real capture writer queued no question for %s", address)
}

func TestAColleaguesImportAfterPersonalSelectionKeepsTheMessage(t *testing.T) {
	e := integration.Setup(t)
	const sender = "new-colleague@example.test"
	mail := seedPurgeableMail(t, e, sender, "Shared correspondence", e.Rep1)
	resolvePersonal(t, e, sender, mail, e.Rep1, false)
	agePersonalMail(t, e, mail, sender, 31*24*time.Hour)
	windows := capture.DefaultPersonalPurgeWindows()
	var selected capture.PurgeSubject
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		floor, _, err := purgeFloorTx(e.Admin(), tx)
		if err != nil {
			return err
		}
		selected, err = capture.SelectPersonalPurgeTx(e.Admin(), tx, e.Rep1, windows, floor, 10)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(selected.SoleImports) != 1 {
		t.Fatalf("selected %+v, want one sole import", selected)
	}
	addImporter(t, e, mail, e.Rep2)
	purger := NewCapturePurger(e.Pool, NewRetentionServiceFor(InstallationDB(e.Pool), nil, slog.Default()))
	n, err := purger.purgePersonalSelection(principal.SystemActing(e.Admin(), personalSweepActor), selected, e.Rep1, windows)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 || activityBody(t, e, mail) == "" {
		t.Fatal("a newly shared message was destroyed")
	}
	if importCount(t, e, mail, e.Rep2) != 1 {
		t.Fatal("the colleague lost their import")
	}
	if importCount(t, e, mail, e.Rep1) != 0 {
		t.Fatal("the personal seat's import was not released")
	}
	if eraseAudits(t, e, mail) != 0 || archiveAuditRows(t, e, mail) != 1 {
		t.Fatal("the changed selection did not audit release instead of destruction")
	}
}

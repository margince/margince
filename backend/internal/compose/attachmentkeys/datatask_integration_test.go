// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package attachmentkeys_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/attachmentkeys"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAttachmentOwnerTaskBoundsRepairAndPreservesUnknownOwners(t *testing.T) {
	e := integration.Setup(t)
	owner := integration.OwnerConn(t)
	contact := e.SeedContact(t, "Buyer", nil)
	live, orphan, unsupported, blocked := ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7()
	pipeline := integration.SeedIDRow(t, owner, `INSERT INTO pipeline(id,name) VALUES($1,'Files')`)
	stage := integration.SeedIDRow(t, owner, `INSERT INTO stage(id,pipeline_id,name,position,semantic,win_probability) VALUES($1,$2,'Open',0,'open',10)`, pipeline)
	deal := integration.SeedIDRow(t, owner, `INSERT INTO deal(id,name,pipeline_id,stage_id,source,captured_by) VALUES($1,'Files',$2,$3,'manual','human:test')`, pipeline, stage)
	room := integration.SeedIDRow(t, owner, `INSERT INTO deal_room(id,deal_id,title,source,captured_by) VALUES($1,$2,'Files','manual','human:test')`, deal)
	ctx := principal.SystemActing(e.Admin(), "attachment-owner-task-test")
	err := pgx.BeginFunc(ctx, owner, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `ALTER TABLE attachment DISABLE TRIGGER trg_attachment_owner_keys;
 ALTER TABLE attachment DROP CONSTRAINT attachment_entity_type_check;
 ALTER TABLE attachment ADD CONSTRAINT attachment_entity_type_check CHECK(entity_type IN ('contact','company','deal','lead','project','activity','offer')) NOT VALID`)
		if err != nil {
			return err
		}
		for _, row := range []struct {
			id    ids.UUID
			kind  string
			owner ids.UUID
		}{{live, "contact", contact}, {orphan, "contact", ids.NewV7()}, {unsupported, "offer", ids.NewV7()}, {blocked, "activity", ids.NewV7()}} {
			if _, err := tx.Exec(ctx, `INSERT INTO attachment(id,entity_type,entity_id,filename,storage_key,source,captured_by,owner_unkeyed,updated_at)
 VALUES($1,$2,$3,'old.txt','old/'||$1::uuid::text,'manual','human:test',true,now()-interval '2 days')`, row.id, row.kind, row.owner); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(ctx, `INSERT INTO deal_room_document(room_id,attachment_id,group_key,title,source,captured_by) VALUES($1,$2,'commercial','Older shared file','manual','human:test')`, room, blocked); err != nil {
			return err
		}

		_, err = tx.Exec(ctx, `ALTER TABLE attachment ENABLE TRIGGER trg_attachment_owner_keys;
 ALTER TABLE attachment DROP CONSTRAINT attachment_entity_type_check;
 SELECT 1`)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	step := attachmentkeys.DataTask().Steps[0]
	in := datatask.BatchIn{Limit: 1, RunID: ids.NewV7()}
	scanned, changed, reported := 0, 0, 0
	for range 5 {
		var out datatask.BatchOut
		if err := pgx.BeginFunc(ctx, owner, func(tx pgx.Tx) error { var err error; out, err = step.Batch(ctx, tx, in); return err }); err != nil {
			t.Fatal(err)
		}
		if out.Scanned > 1 {
			t.Fatalf("batch scanned %d rows with limit 1", out.Scanned)
		}
		scanned += out.Scanned
		changed += out.Changed
		if out.Action != nil {
			for _, record := range out.Action.Records {
				if record.Type != "attachment" || (record.ID != unsupported && record.ID != blocked) {
					t.Fatalf("wrong review record: %+v", record)
				}
				reported++
			}
		}
		if out.Done {
			break
		}
		in.Cursor = out.Cursor
	}
	if scanned != 4 || changed != 2 || reported != 2 {
		t.Fatalf("scanned=%d changed=%d reported=%d; want 4,2,2", scanned, changed, reported)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM attachment WHERE id=$1 AND NOT owner_unkeyed AND owner_contact_id=$2 AND updated_at<now()-interval '1 day'`, live, contact); got != 1 {
		t.Fatal("live file was not keyed without changing its edit time")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM attachment WHERE id=$1`, orphan); got != 0 {
		t.Fatal("orphan file row survived")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM stored_object_intent WHERE storage_key='old/'||$1::uuid::text`, orphan); got != 1 {
		t.Fatal("orphan file was not queued for cleanup")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM attachment WHERE id=$1 AND owner_unkeyed`, unsupported); got != 1 {
		t.Fatal("unsupported file was changed or deleted")
	}
	if err := pgx.BeginFunc(context.Background(), owner, func(tx pgx.Tx) error { _, err := step.Batch(e.Admin(), tx, datatask.BatchIn{Limit: 1}); return err }); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("human ran repair: %v", err)
	}
	// Old unsupported rows remain editable, and a refile is keyed by a retry.
	e.WsExec(t, `UPDATE attachment SET archived_at=now() WHERE id=$1`, unsupported)
	e.WsExec(t, `UPDATE attachment SET entity_type='contact',entity_id=$2 WHERE id=$1`, unsupported, contact)
	if got := e.WsCount(t, `SELECT count(*) FROM attachment WHERE id=$1 AND owner_unkeyed AND owner_contact_id IS NULL`, unsupported); got != 1 {
		t.Fatal("refiling skipped the legacy repair marker")
	}
	var repaired datatask.BatchOut
	if err := pgx.BeginFunc(ctx, owner, func(tx pgx.Tx) error {
		var err error
		repaired, err = step.Batch(ctx, tx, datatask.BatchIn{Limit: 500})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if repaired.Changed != 1 || repaired.Action == nil || len(repaired.Action.Records) != 1 || repaired.Action.Records[0].ID != blocked {
		t.Fatalf("refiled legacy row: %+v", repaired)
	}
	if err := pgx.BeginFunc(ctx, owner, func(tx pgx.Tx) error {
		out, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: 500})
		if err == nil && (out.Changed != 0 || out.Scanned != 1) {
			t.Errorf("replay changed rows: %+v", out)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

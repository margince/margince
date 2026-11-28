// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/storedobjects"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (e *sendEnv) execAttachment(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), sql, args...); err != nil {
		t.Fatal(err)
	}
}

func seedOwnerAttachment(t *testing.T, e *sendEnv, kind string, owner ids.UUID, key string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	e.execAttachment(t, `INSERT INTO attachment(id,entity_type,entity_id,filename,storage_key,source,captured_by,owner_unkeyed,created_at,updated_at) VALUES($1,$2,$3,'review.txt',$4,'manual','human:test',true,now()-interval '2 days',now()-interval '2 days')`, id, kind, owner, key)
	return id
}

func TestEveryAttachmentOwnerKeyRefusesMissingParentsAndCascades(t *testing.T) {
	e := setupSend(t)
	lead, activity := ids.NewV7(), ids.NewV7()
	e.execAttachment(t, `INSERT INTO lead(id,full_name,source,captured_by) VALUES($1,'Buyer','manual','human:test')`, lead)
	e.execAttachment(t, `INSERT INTO activity(id,kind,occurred_at,source,captured_by) VALUES($1,'note',now(),'manual','human:test')`, activity)
	owners := []struct {
		kind string
		id   ids.UUID
	}{
		{"contact", seedCustomer(t, e)},
		{"company", e.seedCompany(t)},
		{"deal", e.seedDeal(t, "Renewal")},
		{"lead", lead},
		{"project", e.seedProject(t, "Renewal")},
		{"activity", activity},
	}
	for _, owner := range owners {
		t.Run(owner.kind, func(t *testing.T) {
			ctx := context.Background()
			_, err := e.owner.Exec(ctx, `INSERT INTO attachment(entity_type,entity_id,filename,storage_key,source,captured_by) VALUES($1,$2,'missing.txt','missing','manual','human:test')`, owner.kind, ids.NewV7())
			fault, ok := errors.AsType[*pgconn.PgError](err)
			if !ok || fault.Code != "23503" || fault.ConstraintName != "attachment_owner_"+owner.kind+"_id_fkey" {
				t.Fatalf("missing %s parent: %v", owner.kind, err)
			}
			key := "test/attachment/" + owner.kind
			id := seedOwnerAttachment(t, e, owner.kind, owner.id, key)
			var keyed bool
			if err := e.owner.QueryRow(ctx, fmt.Sprintf(`SELECT NOT owner_unkeyed AND owner_%s_id=$1 FROM attachment WHERE id=$2`, owner.kind), owner.id, id).Scan(&keyed); err != nil || !keyed {
				t.Fatalf("owner not keyed: %v", err)
			}
			e.execAttachment(t, `DELETE FROM `+owner.kind+` WHERE id=$1`, owner.id)
			var rows, intents int
			if err := e.owner.QueryRow(ctx, `SELECT (SELECT count(*) FROM attachment WHERE id=$1),(SELECT count(*) FROM stored_object_intent WHERE storage_key=$2 AND kind='attachment')`, id, key).Scan(&rows, &intents); err != nil {
				t.Fatal(err)
			}
			if rows != 0 || intents != 1 {
				t.Fatalf("rows=%d intents=%d, want 0 and 1", rows, intents)
			}
		})
	}
}

func TestAttachmentDeletionRollsBackItsFileIntentAndKeepsSharedFiles(t *testing.T) {
	e := setupSend(t)
	owner := seedCustomer(t, e)
	key := "test/attachment/shared"
	first := seedOwnerAttachment(t, e, "contact", owner, key)
	seedOwnerAttachment(t, e, "contact", owner, key)
	ctx := context.Background()
	tx, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM attachment WHERE id=$1`, first); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var intents int
	if err := e.owner.QueryRow(ctx, `SELECT count(*) FROM stored_object_intent WHERE storage_key=$1`, key).Scan(&intents); err != nil || intents != 0 {
		t.Fatalf("rolled-back intent=%d, %v", intents, err)
	}
	e.execAttachment(t, `DELETE FROM attachment WHERE id=$1`, first)
	store, system := systemStore(e)
	unreferenced, err := store.UnreferencedAttachmentKeys(system, []string{key})
	if err != nil || len(unreferenced) != 0 {
		t.Fatalf("shared file became unreferenced: %v, %v", unreferenced, err)
	}
}

func TestDeletingAnAttachmentKeepsAnExistingReaperClaim(t *testing.T) {
	e := setupSend(t)
	owner := seedCustomer(t, e)
	key := "test/attachment/claimed"
	id := seedOwnerAttachment(t, e, "contact", owner, key)
	e.execAttachment(t, `INSERT INTO stored_object_intent(storage_key,kind,recorded_at,reaping_since) VALUES($1,'attachment',now()-interval '2 days',now()-interval '1 hour')`, key)
	e.execAttachment(t, `DELETE FROM attachment WHERE id=$1`, id)
	var kept bool
	if err := e.owner.QueryRow(context.Background(), `SELECT reaping_since IS NOT NULL AND recorded_at>now()-interval '1 minute' FROM stored_object_intent WHERE storage_key=$1`, key).Scan(&kept); err != nil || !kept {
		t.Fatalf("reaper claim changed: %v", err)
	}
}

func TestDeletingTheLastSharedFileReferenceDefeatsAStaleRetire(t *testing.T) {
	e := setupSend(t)
	owner := seedCustomer(t, e)
	key := "test/attachment/retire-race"
	first := seedOwnerAttachment(t, e, "contact", owner, key)
	last := seedOwnerAttachment(t, e, "contact", owner, key)
	e.execAttachment(t, `DELETE FROM attachment WHERE id=$1`, first)
	e.execAttachment(t, `UPDATE stored_object_intent SET recorded_at=$2 WHERE storage_key=$1`, key, clocktest.Now(t).Add(-48*time.Hour))
	store, ctx := systemStore(e)
	ledger := storedobjects.NewLedger(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)))
	pending, err := ledger.ListProvisional(ctx, clocktest.Now(t), 10)
	if err != nil || len(pending) != 1 {
		t.Fatalf("pending=%v, %v", pending, err)
	}
	if free, err := store.UnreferencedAttachmentKeys(ctx, []string{key}); err != nil || len(free) != 0 {
		t.Fatalf("the reaper did not see the remaining file: %v, %v", free, err)
	}
	// The reaper saw the last row, then that row was deleted before Retire.
	e.execAttachment(t, `DELETE FROM attachment WHERE id=$1`, last)
	if err := ledger.Retire(ctx, pending[0]); err != nil {
		t.Fatal(err)
	}
	var kept bool
	if err := e.owner.QueryRow(context.Background(), `SELECT EXISTS(SELECT 1 FROM stored_object_intent WHERE storage_key=$1)`, key).Scan(&kept); err != nil || !kept {
		t.Fatalf("last file lost its cleanup intent: %v", err)
	}
}

func TestReowningAKeyedAttachmentMovesItsKeyAndEditTime(t *testing.T) {
	e := setupSend(t)
	contact, company := seedCustomer(t, e), e.seedCompany(t)
	id := seedOwnerAttachment(t, e, "contact", contact, "test/attachment/refile")
	var before time.Time
	if err := e.owner.QueryRow(context.Background(), `SELECT updated_at FROM attachment WHERE id=$1`, id).Scan(&before); err != nil {
		t.Fatal(err)
	}
	var moved bool
	if err := e.owner.QueryRow(context.Background(), `UPDATE attachment SET entity_type='company',entity_id=$2 WHERE id=$1
 RETURNING owner_contact_id IS NULL AND owner_company_id=$2 AND NOT owner_unkeyed AND updated_at>$3`, id, company, before).Scan(&moved); err != nil || !moved {
		t.Fatalf("refile did not move key and edit time: %v", err)
	}
}

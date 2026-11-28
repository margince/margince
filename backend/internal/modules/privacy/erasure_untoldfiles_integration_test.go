// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestUntoldLeadErasureScrubsFilesBeforeItsParentCascades(t *testing.T) {
	env := setupSARIdentifiers(t)
	lead := seedPlainLead(t, env, "Untold buyer")
	activity, delivery := ids.NewV7(), ids.NewV7()
	ctx := context.Background()
	mustExec(ctx, t, env.owner, `INSERT INTO activity(id,kind,subject,body,source,captured_by) VALUES($1,'email','Notice','Privacy notice','manual','system:test')`, activity)
	mustExec(ctx, t, env.owner, `INSERT INTO activity_link(activity_id,entity_type,lead_id) VALUES($1,'lead',$2)`, activity, lead)
	mustExec(ctx, t, env.owner, `INSERT INTO comms_outbound(id,activity_id,provider,message_id,recipients,cc,subject,body,references_chain,sender_kind,template_key,template_version)
 VALUES($1,$2,'operator_relay','notice@test','["buyer@example.test"]','[]','Notice','Privacy notice','[]','controller','privacy_notice',1)`, delivery, activity)
	mustExec(ctx, t, env.owner, `INSERT INTO lead_privacy_notice(lead_id,delivery_id,template_key,template_version,state,failure,failed_at)
 VALUES($1,$2,'site_lead',1,'failed','bounced',now())`, lead, delivery)
	for _, file := range []struct {
		kind  string
		owner ids.UUID
	}{{"lead", lead}, {"activity", activity}} {
		id := ids.NewV7()
		mustExec(ctx, t, env.owner, `INSERT INTO attachment(id,entity_type,entity_id,filename,storage_key,source,captured_by)
 VALUES($1,$2,$3,'private-buyer-file.txt','untold/'||$1::uuid::text,'manual','human:test')`, id, file.kind, file.owner)
		mustExec(ctx, t, env.owner, `INSERT INTO audit_log(actor_type,actor_id,action,entity_type,entity_id,after,occurred_at)
 VALUES('human','human:test','create','attachment',$1,'{"filename":"private-buyer-file.txt"}',now()-interval '1 hour')`, id)
	}
	in := UntoldLead{LeadID: ids.From[ids.LeadKind](lead), DeliveryID: delivery}
	erase := func() error {
		return env.db.Tx(env.ctx, func(tx pgx.Tx) error { return EraseUntoldLeadTx(env.ctx, tx, in) })
	}
	if err := erase(); err == nil || !strings.Contains(err.Error(), "no object store") {
		t.Fatalf("erasure without a file store: %v", err)
	}
	var count int
	if err := env.owner.QueryRow(ctx, `SELECT count(*) FROM attachment WHERE filename='private-buyer-file.txt'`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("refused erasure changed files: %d, %v", count, err)
	}
	in.Blob = blobstore.NewMemory()
	if err := erase(); err != nil {
		t.Fatal(err)
	}
	for what, query := range map[string]string{
		"file rows":    `SELECT count(*) FROM attachment WHERE filename='private-buyer-file.txt'`,
		"file history": `SELECT count(*) FROM audit_log WHERE coalesce(before::text,'')||coalesce(after::text,'') LIKE '%private-buyer-file.txt%'`,
	} {
		if err := env.owner.QueryRow(ctx, query).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s survived: %d, %v", what, count, err)
		}
	}
	if err := env.owner.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE entity_type='attachment' AND action='erase'`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("file tombstones=%d, %v", count, err)
	}
	if err := env.owner.QueryRow(ctx, `SELECT count(*) FROM stored_object_intent WHERE storage_key LIKE 'untold/%'`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("file cleanup intents=%d, %v", count, err)
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

import (
	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"testing"
)

func TestErasureScrubsLegacyHiddenAddressesAndTheirExceptionExplanations(t *testing.T) {
	e := setupSARIdentifiers(t)
	var address string
	if err := e.owner.QueryRow(e.ctx, `SELECT email FROM contact_email WHERE contact_id=@contact AND archived_at IS NULL LIMIT 1`, pgx.StrictNamedArgs{"contact": e.contact}).Scan(&address); err != nil {
		t.Fatal(err)
	}
	legacy := address[:1] + "\u200b" + address[1:]
	review, instruction := ids.NewV7(), ids.NewV7()
	mustExec(e.ctx, t, e.owner, `INSERT INTO communication_review(id,state,reason_code,refusals) VALUES(@id,'needs_repair','no_marketing_consent',jsonb_build_array(jsonb_build_object('address',@address::text,'reason_code','no_marketing_consent')))`, pgx.StrictNamedArgs{"id": review, "address": legacy})
	mustExec(e.ctx, t, e.owner, `INSERT INTO communication_instruction(id,review_id,directed_by,reason_code,explanation,warning_version,acknowledged_at,facts_as_of,valid_until,given_by_admin)
 VALUES(@id,@review,(SELECT id FROM app_user LIMIT 1),'other','The subject asked on a call.','override-v1',now(),now(),now()+interval '1 day',true)`, pgx.StrictNamedArgs{"id": instruction, "review": review})
	if err := NewEraser(e.db).EraseContact(e.ctx, e.contact.UUID, "request"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := e.owner.QueryRow(e.ctx, `SELECT jsonb_array_length(refusals) FROM communication_review WHERE id=@id`, pgx.StrictNamedArgs{"id": review}).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("legacy refusal still identifies the erased subject")
	}
	var explanation string
	if err := e.owner.QueryRow(e.ctx, `SELECT explanation FROM communication_instruction WHERE id=@id`, pgx.StrictNamedArgs{"id": instruction}).Scan(&explanation); err != nil {
		t.Fatal(err)
	}
	if explanation != "[erased]" {
		t.Fatalf("exception retained personal explanation: %q", explanation)
	}
}

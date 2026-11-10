// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/originalstore"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestMailCopyRepairKeepsTheIdentityOfAnOffloadedOriginal(t *testing.T) {
	e := integration.Setup(t)
	blob := &failingOriginalBlob{Store: blobstore.NewMemory()}
	ctx := originalstore.Bind(e.Admin(), e.DB(), blob)
	const message = "original-identity@example.test"
	payload, err := json.Marshal(map[string]string{"rfc_message_id": message, "body": strings.Repeat("x", originalstore.InlineLimit)})
	if err != nil {
		t.Fatal(err)
	}
	var original ids.UUID
	if err := e.DB().Tx(ctx, func(tx pgx.Tx) error {
		var err error
		original, err = capture.InsertRawCaptureTx(ctx, tx, capture.RawRecord{SourceSystem: "legacy", SourceID: "copy", Payload: payload})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	kept, duplicate := ids.NewV7(), ids.NewV7()
	// Legacy imports did not retain activity identity evidence beside the original.
	e.WsExec(t, `INSERT INTO activity(id,kind,subject,occurred_at,source,captured_by,raw_capture_id)
 VALUES($1,'email','Kept',now(),'manual','human:test',NULL),($2,'email','Copy',now(),'manual','human:test',$3)`, kept, duplicate, original)
	e.WsExec(t, `INSERT INTO activity_identity(identity_kind,identity_key,activity_id,attested_by) VALUES('mail',$1,$2,'connector:fixture')`, message, kept)
	system := principal.SystemActing(ctx, "mail_copy_repair")
	if err := e.DB().Tx(system, func(tx pgx.Tx) error {
		pairs, err := activities.MailCopiesTx(system, tx, ids.Nil, 10)
		if err != nil {
			return err
		}
		if len(pairs) != 1 || pairs[0].ID != duplicate || pairs[0].Kept != kept {
			t.Fatalf("offloaded message identity was lost: %+v", pairs)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if blob.gets.Load() != 0 {
		t.Fatal("identity lookup downloaded the full original")
	}
}

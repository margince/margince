// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

type countedFileStage struct {
	capture.FileKeeper
	calls int
}

func (k *countedFileStage) Stage(ctx context.Context, files []capture.CapturedFile) ([]capture.StagedFile, error) {
	k.calls++
	return k.FileKeeper.Stage(ctx, files)
}

func TestAnAbortedThreadJoinReusesItsStagedAttachment(t *testing.T) {
	o := setupOwed(t)
	const rootA, rootB, reply, recipient = "file-root-a@myco.example", "file-root-b@myco.example", "file-retry@myco.example", "buyer@customer.example"
	o.weWrite(t, owedMail{to: recipient, subject: "Agreement", messageID: rootA, body: "First message.", at: o.now.Add(-3 * time.Hour)})
	o.weWrite(t, owedMail{to: recipient, subject: "Agreement", messageID: rootB, body: "Other message.", at: o.now.Add(-2 * time.Hour)})
	raw := []byte(fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: Agreement\r\nDate: %s\r\nMessage-ID: <%s>\r\nReferences: <%s> <%s>\r\nMIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=retry-parts\r\n\r\n--retry-parts\r\nContent-Type: text/plain\r\n\r\nThe attachment.\r\n--retry-parts\r\nContent-Type: application/pdf\r\nContent-Disposition: attachment; filename=contract.pdf\r\n\r\n%s\r\n--retry-parts--\r\n", o.seat, recipient, o.now.Format(time.RFC1123Z), reply, rootA, rootB, onePDF().Body))
	parsed, err := mailmap.Parse(raw, o.seat)
	if err != nil {
		t.Fatal(err)
	}
	parsed = parsed.AttestSentByOwner(true)
	record := parsed.ToRecord("gmail", raw)
	if len(record.Parts) != 1 {
		t.Fatalf("parsed attachments=%d, want one", len(record.Parts))
	}
	keeper := &countedFileStage{FileKeeper: fileKeeper(o.e.Pool, blobstore.NewMemory())}
	joiner := threadJoiner()
	merge := joiner.Merge
	attempts := 0
	joiner.Merge = func(ctx context.Context, tx pgx.Tx, from, to string) error {
		attempts++
		if attempts == 1 {
			return storekit.RetryThreadTransaction(ctx, tx)
		}
		return merge(ctx, tx, from, to)
	}
	sink := newCaptureSink(o.e.Pool, CaptureConfig{}).WithFileKeeper(keeper).WithThreadJoin(joiner)
	ctx := threadConnectorCtx(o.e, o.owner)
	if _, err := sink.Upsert(ctx, record); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 || keeper.calls != 1 {
		t.Fatalf("merge attempts=%d file stages=%d, want two attempts and one stage", attempts, keeper.calls)
	}
	if files := filesFor(ctx, t, o.e.DB(), reply); len(files) != 1 {
		t.Fatalf("committed files=%d, want one", len(files))
	}
	if rows := o.e.WsCount(t, `SELECT count(*) FROM activity WHERE source_system='email' AND source_id=$1`, reply); rows != 1 {
		t.Fatalf("committed message copies=%d, want one", rows)
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
)

func TestARepairDoesNotRetireTheConfidentialityQuestionAsMissingEvidence(t *testing.T) {
	e := integration.Setup(t)
	const message = "confidentiality-question@counterparty.example"
	captureThreadMail(t, e, e.Rep1, subjectThreadMail(seatAddress(t, e, e.Rep1), message, "Price", ""))
	id := capturedThreadActivity(t, e, message)
	key := threadKeyOfMessage(t, e, message)
	store := capture.NewThreadVerdictStore(e.DB())
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if err := store.EnsureTx(e.Admin(), tx, key, e.Rep1, id.UUID, time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)); err != nil {
			return err
		}
		_, err := tx.Exec(e.Admin(), `UPDATE capture_thread_verdict SET updated_at='2000-01-01' WHERE thread_key=@key AND user_id=@user`, pgx.NamedArgs{"key": key, "user": e.Rep1})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	run := beginUnavailableThread(t, e, key)
	retired, err := store.RetireExhausted(e.Admin(), "attempts exhausted")
	if err != nil || retired != 0 {
		t.Fatalf("repair retired %d questions: %v", retired, err)
	}
	if got := e.WsScalar(t, `SELECT status FROM capture_thread_verdict WHERE thread_key=$1 AND user_id=$2`, key, e.Rep1); got != "pending" {
		t.Fatalf("question during repair=%s", got)
	}
	finishUnavailableThread(t, e, run)
	if got := e.WsScalar(t, `SELECT status FROM capture_thread_verdict WHERE thread_key=$1 AND user_id=$2`, key, e.Rep1); got != "pending" {
		t.Fatalf("question after repair=%s", got)
	}
}

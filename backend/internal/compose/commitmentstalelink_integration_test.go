// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A promise somebody settled or dismissed after a task was proposed for it is
// not given that open task when the proposal is accepted later.
func TestASettledPromiseCannotAcquireAnOpenTask(t *testing.T) {
	for _, outcome := range []string{claimStatusDone, "dismissed"} {
		t.Run(outcome, func(t *testing.T) {
			k := seedKeptCommitment(t)
			recipient := k.SeedContact(t, "Later Recipient", &k.Rep2)
			quote := k.wsString(t, `SELECT source_quote FROM conversation_claim WHERE id=$1`, k.claim)
			locator := k.wsString(t, `SELECT evidence_fingerprint FROM conversation_claim WHERE id=$1`, k.claim)
			claim, err := k.Contacts.RecordConversationClaim(k.Admin(), contacts.ClaimInput{
				ContactID: ids.From[ids.ContactKind](recipient), Kind: claimKindOurs, Body: "Send the agreed material",
				ActivityID: k.activity.UUID, Quote: quote, Source: "test", Locator: locator,
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := k.Contacts.SettleConversationClaim(k.Admin(), ids.UUID(claim.Id), outcome); err != nil {
				t.Fatal(err)
			}
			err = InstallationDB(k.Pool).Tx(k.Admin(), func(tx pgx.Tx) error {
				return k.Contacts.SetClaimTaskTx(k.Admin(), tx, ids.UUID(claim.Id), k.task)
			})
			if !errors.Is(err, apperrors.ErrConflict) {
				t.Fatalf("linking a %s promise to open work: %v", outcome, err)
			}
			if linked := k.WsCount(t, `SELECT count(*) FROM conversation_claim WHERE id=$1 AND task_activity_id IS NOT NULL`, ids.UUID(claim.Id)); linked != 0 {
				t.Fatal("the settled promise was linked to an open task")
			}
		})
	}
}

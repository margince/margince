// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const clonedPurposeDeliverySQL = `INSERT INTO comms_outbound
 SELECT (jsonb_populate_record(original,jsonb_build_object('id',$2::uuid,'consent_purpose',$3::text,'message_id',$2::uuid::text))).*
 FROM comms_outbound original WHERE id=$1`

func TestAnArchivedPurposeAllowsOnlyRefusalDecisions(t *testing.T) {
	e := setupChannelConsent(t)
	ctx := purposeConfigurer(e)
	template := plantConsentDelivery(t, e.owner, e.user)
	purpose, err := e.store.CreatePurpose(ctx, "retired_news", "Retired news", false, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.store.ArchivePurpose(ctx, purpose.ID); err != nil {
		t.Fatal(err)
	}
	_, err = e.owner.Exec(ctx, clonedPurposeDeliverySQL, template, ids.NewV7(), purpose.Key)
	assertArchivedPurposeRefused(t, err)
	for _, verdict := range []string{"allow", "deny", "review"} {
		_, err = e.owner.Exec(ctx, `INSERT INTO communication_decision(delivery_id,attempt,decision_set_id,recipient_address,phase,resolved_category,verdict,reason_code,mode,actor,purpose_key) VALUES($1,0,$2,'reader@example.test','staging','marketing',$3,'no_compatible_evidence','observe','test',$4)`, template, ids.NewV7(), verdict, purpose.Key)
		if verdict == "allow" {
			assertArchivedPurposeRefused(t, err)
		} else if err != nil {
			t.Fatalf("%s decision was lost: %v", verdict, err)
		}
	}
}

func assertArchivedPurposeRefused(t *testing.T, err error) {
	t.Helper()
	var refusal *pgconn.PgError
	if !errors.As(err, &refusal) || refusal.ConstraintName != "consent_purpose_in_use" {
		t.Fatalf("archived purpose was not refused: %v", err)
	}
}

func TestADeliveryWaitingOnAnArchiveCannotUseTheRetiredPurpose(t *testing.T) {
	e := setupChannelConsent(t)
	ctx, cancel := context.WithTimeout(purposeConfigurer(e), 30*time.Second)
	defer cancel()
	template := plantConsentDelivery(t, e.owner, e.user)
	purpose, err := e.store.CreatePurpose(ctx, "archive_before_send", "Archive before send", false, "")
	if err != nil {
		t.Fatal(err)
	}
	ready := make(chan int32, 1)
	finish := make(chan struct{})
	archiveResult := make(chan error, 1)
	go func() {
		archiveResult <- database.WithWriteTx(ctx, e.store.db.Pool(), func(joined context.Context) error {
			if err := e.store.ArchivePurpose(joined, purpose.ID); err != nil {
				return err
			}
			if err := e.store.db.Tx(joined, func(tx pgx.Tx) error {
				var pid int32
				if err := tx.QueryRow(joined, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
					return err
				}
				ready <- pid
				return nil
			}); err != nil {
				return err
			}
			select {
			case <-finish:
				return nil
			case <-joined.Done():
				return joined.Err()
			}
		})
	}()
	var archiver int32
	select {
	case archiver = <-ready:
	case err := <-archiveResult:
		t.Fatalf("archive failed before the race: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	returned := make(chan struct{})
	deliveryResult := make(chan error, 1)
	go func() {
		deliveryResult <- e.store.db.Tx(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, clonedPurposeDeliverySQL, template, ids.NewV7(), purpose.Key)
			return err
		})
		close(returned)
	}()
	testdb.AwaitQueuedBehind(t, e.owner, archiver, 1, returned, "allowing delivery")
	close(finish)
	if err := <-archiveResult; err != nil {
		t.Fatal(err)
	}
	assertArchivedPurposeRefused(t, <-deliveryResult)
}

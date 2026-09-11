// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Fulfilling an erasure case that a data subject opened THEMSELVES, through the
// public confirm link, against the real privacy.Eraser.
//
// The suite beside this drives cases made by an officer through CreateDSR, and
// those carry no source_submission_id. A case opened from a confirm link does,
// and that reference is what makes this path different: the erasure deletes the
// submission the case points at, so the delete has to reach the case row too.
//
// FulfilErasure holds that row under FOR UPDATE for the whole erase — the
// guarantee TestFulfillErasureHoldsTheRequestLockedAcrossTheErase exists to
// pin — and the erase runs in its own transaction. A foreign key that made the
// delete update the locked row would have the fulfilment wait on a lock it is
// holding itself, which is a wait nothing can end.

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedSubmissionBackedErasureCase opens an erasure case the way the public
// confirm edge does: a submission row first, then a case naming it.
//
// Written here rather than driven through SubmitConfirmation because that path
// needs a live confirm token, and what this test turns on is the reference
// between the two rows, not how they came to exist.
func seedSubmissionBackedErasureCase(t *testing.T, e *Env, personID ids.UUID) ids.UUID {
	t.Helper()
	ctx := context.Background()
	var tokenID, submissionID, caseID ids.UUID
	pool := e.Pool
	if err := pool.QueryRow(ctx, `
		INSERT INTO confirm_token (person_id, token_hash, delivered_to, expires_at)
		VALUES ($1, $2, $3, now() + interval '30 days')
		RETURNING id`, personID, "erasure-case-"+personID.String(),
		"subject-"+personID.String()+"@erasure.test").Scan(&tokenID); err != nil {
		t.Fatalf("seeding the confirm token: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO person_confirm_submission (person_id, token_id, kind)
		VALUES ($1, $2, 'erasure_request')
		RETURNING id`, personID, tokenID).Scan(&submissionID); err != nil {
		t.Fatalf("seeding the subject's removal request: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO data_subject_request
		  (kind, subject_ref, person_id, received_at, channel, due_at,
		   source_submission_id, receipt_reference)
		VALUES ('erasure', $1, $2, now(), 'confirm_link', now() + interval '1 month',
		        $3, $4)
		RETURNING id`, personID.String(), personID, submissionID,
		"DSR-"+personID.String()[:10]).Scan(&caseID); err != nil {
		t.Fatalf("seeding the rights case: %v", err)
	}
	return caseID
}

// TestFulfillingAnErasureCaseTheSubjectOpenedCompletes is the whole point: a
// subject who asks to be removed through their own link must actually be
// removable.
//
// The deadline is what makes the assertion honest. A self-deadlock does not
// fail, it WAITS — the fulfilment blocks on a row it holds itself — so without
// a bound this test would hang the suite instead of reporting anything.
func TestFulfillingAnErasureCaseTheSubjectOpenedCompletes(t *testing.T) {
	e := Setup(t)
	personID := e.SeedPerson(t, "Self-Requesting Subject", nil)
	caseID := seedSubmissionBackedErasureCase(t, e, personID)

	h := consent.NewHandlers(e.DB()).WithEraser(privacy.NewEraser(e.DB()))

	done := make(chan int, 1)
	go func() {
		w := fulfilErasureDSR(t, e, h, caseID, `{"status":"fulfilled","resolution":"erased on request"}`)
		done <- w.Code
	}()

	select {
	case code := <-done:
		if code != http.StatusOK {
			t.Fatalf("fulfilling the subject's own erasure request answered %d, want 200", code)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("fulfilling an erasure case the subject opened never finished — the erase deletes " +
			"the submission the case points at, and the case row FulfilErasure holds locked is the " +
			"one that delete has to touch, so the fulfilment waits on itself and no subject who " +
			"asked through their own link can ever be erased")
	}
}

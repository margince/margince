// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// owedNoticeCase opens an Art. 14 duty on the contact, as capture does for a
// contact whose source it cannot name, and answers the case id.
func owedNoticeCase(t *testing.T, e *integration.Env, contactID ids.ContactID) ids.UUID {
	t.Helper()
	var caseID ids.UUID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			WITH ev AS (
			  INSERT INTO contact_acquisition_evidence (contact_id, kind, occurred_at, captured_by)
			  VALUES ($1, 'unknown_legacy', now(), 'connector:gmail') RETURNING id)
			INSERT INTO privacy_notice_case (contact_id, acquisition_id, rule, due_at, state)
			SELECT $1, ev.id, 'art14', now() + interval '1 month', 'open' FROM ev RETURNING id`,
			contactID.UUID).Scan(&caseID)
	}); err != nil {
		t.Fatalf("opening the notice duty: %v", err)
	}
	return caseID
}

func noticeCaseState(t *testing.T, e *integration.Env, caseID ids.UUID) string {
	t.Helper()
	var state string
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT state FROM privacy_notice_case WHERE id = $1`, caseID).Scan(&state)
	}); err != nil {
		t.Fatalf("reading the notice duty: %v", err)
	}
	return state
}

// A withdrawn contact is no longer processed as business data, so the duty it
// owed must not stay on the worklist with a deadline.
func TestAPersonalVerdictEndsTheNoticeDutyOfTheContactItWithdraws(t *testing.T) {
	e := integration.Setup(t)
	const aunt = "aunt@family.test"
	activityID := seedHeldThreadMail(t, e, "thread-family", aunt, "Geburtstag")
	threadID := seedThreadQuestion(t, e, "thread-family", activityID)
	contactID := seedCapturedContact(t, e, aunt)
	caseID := owedNoticeCase(t, e, contactID)

	runConfidentiality(t, e, threadID, confidentialityPersonal, 0.95)

	if got := noticeCaseState(t, e, caseID); got != "exempt_with_reason" {
		t.Errorf("the withdrawn contact's notice duty is %q, want exempt_with_reason", got)
	}
}

// A contact the verdict keeps keeps its duty too.
func TestAPersonalVerdictLeavesTheDutyOfAContactItKeeps(t *testing.T) {
	e := integration.Setup(t)
	const both = "cousin@family.test"
	businessActivity := seedHeldThreadMail(t, e, "thread-business", both, "Angebot")
	runConfidentiality(t, e, seedThreadQuestion(t, e, "thread-business", businessActivity), confidentialityOrdinary, 0.95)
	privateActivity := seedHeldThreadMail(t, e, "thread-private", both, "Familie")
	threadID := seedThreadQuestion(t, e, "thread-private", privateActivity)
	contactID := seedCapturedContact(t, e, both)
	caseID := owedNoticeCase(t, e, contactID)

	runConfidentiality(t, e, threadID, confidentialityPersonal, 0.95)

	if got := noticeCaseState(t, e, caseID); got != "open" {
		t.Errorf("a kept contact's notice duty is %q, want open", got)
	}
}

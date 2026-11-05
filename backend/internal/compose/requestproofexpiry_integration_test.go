// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// answerProofAt moves a proof's answer date, and its retention date by the
// writer's own rule. The sweep then meets ages a test cannot wait for.
func (e proofEnv) answerProofAt(t *testing.T, request ids.UUID, answered time.Time) {
	t.Helper()
	e.WsExec(t, `UPDATE privacy_request_proof SET answered_at = $2, keep_until = $3 WHERE request_id = $1`,
		request, answered, consent.ProofKeepUntil(answered, e.installationZone(t)))
}

func (e proofEnv) closeAccess(t *testing.T, subjectRef string) ids.UUID {
	t.Helper()
	request := e.openRequest(t, "access", subjectRef)
	e.patch(t, request, `{"status":"fulfilled","resolution":"package sent","identity_check":"address_on_file"}`)
	return request
}

// expire runs the pass the worker registers, at now.
func (e proofEnv) expire(t *testing.T, now time.Time) {
	t.Helper()
	worker := newRequestProofExpiryWorker(e.Pool, func() time.Time { return now }, slog.New(slog.DiscardHandler))
	if err := worker.Work(context.Background(), nil); err != nil {
		t.Fatalf("expiry pass: %v", err)
	}
}

func TestTheJanuaryRunDeletesProofsThreeYearsAfterTheirYearAndKeepsTheRest(t *testing.T) {
	e := setupProofs(t)
	due := e.closeAccess(t, "due@proof.test")
	younger := e.closeAccess(t, "younger@proof.test")
	disputed := e.closeAccess(t, "disputed@proof.test")
	e.answerProofAt(t, due, time.Date(2026, 12, 31, 9, 0, 0, 0, time.UTC))
	e.answerProofAt(t, younger, time.Date(2027, 1, 1, 9, 0, 0, 0, time.UTC))
	e.answerProofAt(t, disputed, time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC))
	e.patch(t, disputed, `{"dispute_open":true}`)

	e.expire(t, time.Date(2029, 12, 31, 23, 0, 0, 0, time.UTC))
	if n := e.proofCount(t, due); n != 1 {
		t.Fatal("a proof went before its third year had ended")
	}
	e.expire(t, time.Date(2030, 2, 1, 0, 0, 0, 0, time.UTC))
	if n := e.proofCount(t, due); n != 1 {
		t.Fatal("a run outside January deleted a proof")
	}
	e.expire(t, time.Date(2030, 1, 2, 3, 0, 0, 0, time.UTC))
	if n := e.proofCount(t, due); n != 0 {
		t.Fatal("the January run kept a proof three years past the end of its year")
	}
	if n := e.proofCount(t, younger); n != 1 {
		t.Fatal("the January run deleted a proof answered a day later, in the next year")
	}
	if n := e.proofCount(t, disputed); n != 1 {
		t.Fatal("the January run deleted a proof whose request is under dispute")
	}

	e.patch(t, disputed, `{"dispute_open":false}`)
	e.expire(t, time.Date(2030, 1, 20, 3, 0, 0, 0, time.UTC))
	if n := e.proofCount(t, disputed); n != 0 {
		t.Fatal("a proof outlived the dispute that held it")
	}
}

func TestAnExpiredProofTakesTheRequestersReferenceFromTheRequestAndItsAuditImage(t *testing.T) {
	e := setupProofs(t)
	const reference = "expiring.requester@proof.test"
	request := e.closeAccess(t, reference)
	e.answerProofAt(t, request, time.Date(2025, 3, 1, 9, 0, 0, 0, time.UTC))

	entity, id := "data_subject_request", request
	readTrail := func() string {
		page, err := privacy.ListAuditLog(e.Admin(), e.DB(), privacy.AuditFilter{EntityType: &entity, EntityID: &id})
		if err != nil {
			t.Fatal(err)
		}
		var trail strings.Builder
		for _, entry := range page.Entries {
			trail.Write(entry.Before)
			trail.Write(entry.After)
		}
		return trail.String()
	}
	if !strings.Contains(readTrail(), reference) {
		t.Fatal("the creation image never held the reference, so its absence below would prove nothing")
	}

	e.expire(t, time.Date(2029, 1, 3, 3, 0, 0, 0, time.UTC))

	if got := e.WsScalar(t, `SELECT subject_ref FROM data_subject_request WHERE id = $1`, request); got != "erased+"+request.String() {
		t.Fatalf("the expired request still names %q", got)
	}
	if trail := readTrail(); strings.Contains(trail, reference) {
		t.Fatalf("the audit trail still shows the reference: %s", trail)
	}
}

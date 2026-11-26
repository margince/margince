// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package finance

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/ratelimit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func readerCtx() context.Context {
	return humanCtx(ids.NewV7())
}

func humanCtx(user ids.UUID) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
	})
}

// agentCtx is an agent acting for user under a passport of its own, which a
// refresh replaces with a new one.
func agentCtx(user ids.UUID) context.Context {
	passport := ids.NewV7()
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:" + passport.String(), UserID: user,
		PassportID: passport, OnBehalfOf: user,
	})
}

func TestACallerPastTwentyDocumentsAMinuteIsRefused(t *testing.T) {
	reader := readerCtx()
	for i := range invoiceDocumentsPerMinute {
		if err := admitDocumentFetch(reader); err != nil {
			t.Fatalf("fetch %d of %d refused: %v", i+1, invoiceDocumentsPerMinute, err)
		}
	}
	if err := admitDocumentFetch(reader); !errors.Is(err, apperrors.ErrBudgetExceeded) {
		t.Errorf("fetch past the ceiling: %v, want ErrBudgetExceeded", err)
	}
	if err := admitDocumentFetch(readerCtx()); err != nil {
		t.Errorf("another caller was refused: %v; the ceiling is per caller", err)
	}
}

func TestADocumentFetchWithoutACallerIsRefused(t *testing.T) {
	if err := admitDocumentFetch(context.Background()); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a fetch with no caller: %v, want ErrPermissionDenied", err)
	}
}

// A human and every agent acting for them spend one budget. A new passport,
// minted or refreshed, buys no fresh one.
func TestAgentsActingForAHumanShareTheirBudget(t *testing.T) {
	user := ids.NewV7()
	for i := range invoiceDocumentsPerMinute {
		fetch := humanCtx(user)
		if i%2 == 1 {
			fetch = agentCtx(user)
		}
		if err := admitDocumentFetch(fetch); err != nil {
			t.Fatalf("fetch %d of %d refused: %v", i+1, invoiceDocumentsPerMinute, err)
		}
	}
	if err := admitDocumentFetch(agentCtx(user)); !errors.Is(err, apperrors.ErrBudgetExceeded) {
		t.Errorf("an agent under a fresh passport, past its human's ceiling: %v, want ErrBudgetExceeded", err)
	}
}

// A refusal is a 429 that says when to come back, and coming back then works:
// the ceiling lifts when its minute ends.
func TestTheCeilingAnswersRetryAfterAndLiftsWhenItsMinuteEnds(t *testing.T) {
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	was := invoiceDocumentFetches
	invoiceDocumentFetches = ratelimit.NewWithClock("test-finance-invoice-document", ratelimit.FailClosed,
		invoiceDocumentsPerMinute, time.Minute, func() time.Time { return now })
	t.Cleanup(func() { invoiceDocumentFetches = was })

	reader := readerCtx()
	for range invoiceDocumentsPerMinute {
		if err := admitDocumentFetch(reader); err != nil {
			t.Fatalf("a fetch within the ceiling was refused: %v", err)
		}
	}
	fault, ok := httperr.Classify(admitDocumentFetch(reader))
	if !ok || fault.Status != http.StatusTooManyRequests || fault.Code != "rate_limited" || fault.RetryAfterSeconds != 60 {
		t.Fatalf("the refusal classifies as %+v, want 429 rate_limited with Retry-After 60", fault)
	}
	now = now.Add(time.Minute + time.Second)
	if err := admitDocumentFetch(reader); err != nil {
		t.Errorf("a fetch after the minute ended was refused: %v", err)
	}
}

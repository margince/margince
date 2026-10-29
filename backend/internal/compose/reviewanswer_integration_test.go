// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The four answers a customer review takes. Revived is confirmed against the
// deal's evidence; a wait and closed lost also write the deal, in the same
// transaction as the answer. These drive the real store and handlers over
// deals the real worker judged.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (v *viabilityEnv) reviewStore() *assurance.Store {
	return assurance.NewStore(InstallationDB(v.Pool)).WithReviewCheck(AssuranceReviewCheck)
}

func (v *viabilityEnv) reviewOf(t *testing.T, deal ids.UUID) ids.UUID {
	t.Helper()
	review, err := ids.Parse(v.WsScalar(t, `SELECT id::text FROM assurance_exception
		WHERE type = $1 AND subject_id = $2`, assurance.TypeDealViability, deal))
	if err != nil {
		t.Fatalf("the deal has no review: %v", err)
	}
	return review
}

func (v *viabilityEnv) postAnswer(t *testing.T, review ids.UUID, path string, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	h := reviewAnswerHandlers{pool: v.Pool, deals: v.Deals, assurance: v.reviewStore(), now: func() time.Time { return clocktest.Now(t) }}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/forecast/assurance/exceptions/"+review.String()+"/"+path,
		bytes.NewReader(raw)).WithContext(v.Admin())
	if path == "wait" {
		h.WaitInputCheck(rec, req, openapi_types.UUID(review))
	} else {
		h.CloseLostInputCheck(rec, req, openapi_types.UUID(review))
	}
	return rec
}

func TestARevivalIsRefusedUntilTheCustomerIsBack(t *testing.T) {
	v := setupViability(t)
	v.check(t)
	review := v.reviewOf(t, v.silent)

	err := v.reviewStore().Resolve(v.Admin(), review, assurance.Resolution{Outcome: assurance.OutcomeFixedRecord}, clocktest.Now(t))
	if err == nil || !strings.Contains(err.Error(), "not_revived") && !strings.Contains(err.Error(), "cannot count again") {
		t.Fatalf("reviving a still-silent deal answered %v, want a refusal naming what is missing", err)
	}
	if v.openReviews(t, v.silent) != 1 {
		t.Fatalf("a refused revival closed the review")
	}

	v.customerWrites(t, v.silent, clocktest.Now(t).Add(-time.Hour))
	if err := v.reviewStore().Resolve(v.Admin(), review, assurance.Resolution{Outcome: assurance.OutcomeFixedRecord},
		clocktest.Now(t)); err != nil {
		t.Fatalf("reviving a deal the customer came back to: %v", err)
	}
	if v.openReviews(t, v.silent) != 0 {
		t.Fatalf("the revived review is still open")
	}
}

func TestAWaitIsRecordedOnTheDealAndKeepsTheReview(t *testing.T) {
	v := setupViability(t)
	v.check(t)
	review := v.reviewOf(t, v.silent)
	until := clocktest.Now(t).AddDate(0, 0, 30).Format(time.DateOnly)

	if rec := v.postAnswer(t, review, "wait", map[string]any{"until": until}); rec.Code != http.StatusNoContent {
		t.Fatalf("waiting answered %d %s", rec.Code, rec.Body)
	}
	if got := v.WsScalar(t, `SELECT coalesce(wait_until::text, '') FROM deal WHERE id = $1`, v.silent); got != until {
		t.Errorf("the deal waits until %q, want %s", got, until)
	}
	if v.openReviews(t, v.silent) != 1 {
		t.Errorf("a wait closed the review; it only defers it")
	}
	if got := v.WsScalar(t, `SELECT outcome FROM assurance_resolution WHERE exception_id = $1`, review); got != assurance.OutcomeRemindLater {
		t.Errorf("the review was answered %q, want %s", got, assurance.OutcomeRemindLater)
	}
}

func TestClosingLostClosesTheDealAndTheReview(t *testing.T) {
	v := setupViability(t)
	v.check(t)
	review := v.reviewOf(t, v.silent)
	until := clocktest.Now(t).AddDate(0, 0, 30)
	if err := v.reviewStore().Resolve(v.Admin(), review, assurance.Resolution{
		Outcome: assurance.OutcomeNotRelevant, Reason: "Gone quiet", ExpiresAt: &until,
	}, clocktest.Now(t)); err == nil {
		t.Fatal("a review was dismissed as not relevant while its deal is still open")
	}

	if rec := v.postAnswer(t, review, "close-lost", map[string]any{"lost_reason": "Chose another supplier"}); rec.Code != http.StatusNoContent {
		t.Fatalf("closing lost answered %d %s", rec.Code, rec.Body)
	}
	if got := v.WsScalar(t, `SELECT status FROM deal WHERE id = $1`, v.silent); got != "lost" {
		t.Errorf("the deal is %q after closing lost", got)
	}
	if v.openReviews(t, v.silent) != 0 {
		t.Errorf("the review stayed open after its deal was closed lost")
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestARepConfirmsRecipientPreferencesAndTheProviderReceivesOneEmail(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"subject_request", "marketing_objection", "processing_restriction", "withdrawn"} {
		t.Run(kind, func(t *testing.T) {
			p := setupPreflight(t)
			p.connect(t, apptest.GmailReadonlyScope, apptest.GmailSendScope)
			if kind == "withdrawn" {
				var purpose string
				if err := p.Owner.QueryRow(t.Context(), `SELECT id FROM consent_purpose WHERE key='marketing_email'`).Scan(&purpose); err != nil {
					t.Fatal(err)
				}
				if status := p.Call(t, "POST", "/v1/contacts/"+p.contactID+"/consent", AnyMap{"purpose_id": purpose, "new_state": "withdrawn"}, nil, nil); status != http.StatusOK {
					t.Fatalf("withdrawing=%d", status)
				}
			} else if status := p.Call(t, "POST", "/v1/contacts/"+p.contactID+"/consent/suppress", AnyMap{"kind": kind, "reason": "The recipient recorded this preference."}, nil, nil); status != http.StatusNoContent {
				t.Fatalf("recording preference=%d", status)
			}
			demoteToRep(t, p.AppEnv)
			send := func() int {
				return p.Call(t, "POST", "/v1/activities/"+p.activityID+"/send-email", AnyMap{"subject": "Requested update", "body": "Here is the requested update.", "to": []string{"buyer@preflight.test"}, "consent_purpose": "marketing_email"}, nil, nil)
			}
			if status := send(); status != http.StatusConflict {
				t.Fatalf("send=%d", status)
			}
			c := &consentEnv{AppEnv: p.AppEnv, contactID: p.contactID, activityID: p.activityID}
			reviewID := liveReviewID(t, c)
			var review struct {
				AssessmentRevision string `json:"assessment_revision"`
				ReasonCode         string `json:"reason_code"`
			}
			if status := p.Call(t, "GET", "/v1/communication-reviews/"+reviewID, nil, nil, &review); status != http.StatusOK {
				t.Fatalf("review=%d", status)
			}
			confirmation := directedShowing(review.ReasonCode)
			confirmation["assessment_revision"] = review.AssessmentRevision
			var sent struct {
				ID string `json:"id"`
			}
			if status := p.Call(t, "POST", "/v1/communication-reviews/"+reviewID+"/direct-send", confirmation, nil, &sent); status != http.StatusCreated {
				t.Fatalf("confirmation=%d", status)
			}
			activity, err := ids.Parse(sent.ID)
			if err != nil {
				t.Fatal(err)
			}
			delivery, _ := p.deliveryFor(t, activity)
			p.transmit(t, delivery, "")
			var delivered, unchanged int
			if err := p.Owner.QueryRow(t.Context(), `SELECT count(*) FROM comms_outbound WHERE execution_authority='instruction' AND id=@delivery`, pgx.StrictNamedArgs{"delivery": delivery}).Scan(&delivered); err != nil {
				t.Fatal(err)
			}
			query := `SELECT count(*) FROM communication_suppression WHERE contact_id=@contact AND revoked_at IS NULL`
			if kind == "withdrawn" {
				query = `SELECT count(*) FROM contact_consent WHERE contact_id=@contact AND state='withdrawn'`
			}
			if err := p.Owner.QueryRow(t.Context(), query, pgx.StrictNamedArgs{"contact": p.contactID}).Scan(&unchanged); err != nil {
				t.Fatal(err)
			}
			if delivered != 1 || unchanged != 1 {
				t.Fatalf("deliveries=%d recorded preferences=%d", delivered, unchanged)
			}
			if status := send(); status != http.StatusConflict {
				t.Fatalf("next message=%d", status)
			}
		})
	}
}

func TestASecondStopWithTheSameCodeNeedsAFreshConfirmation(t *testing.T) {
	t.Parallel()
	c := setupConsent(t)
	stop := func() {
		t.Helper()
		if status := c.Call(t, "POST", "/v1/contacts/"+c.contactID+"/consent/suppress", AnyMap{"kind": "subject_request", "reason": "The recipient asked us to stop."}, nil, nil); status != http.StatusNoContent {
			t.Fatalf("stop=%d", status)
		}
	}
	stop()
	if status, _ := c.send(t, "marketing_email"); status != http.StatusConflict {
		t.Fatalf("send=%d", status)
	}
	oldID := liveReviewID(t, c)
	var old struct {
		AssessmentRevision string `json:"assessment_revision"`
	}
	if status := c.Call(t, "GET", "/v1/communication-reviews/"+oldID, nil, nil, &old); status != http.StatusOK {
		t.Fatalf("review=%d", status)
	}
	stop()
	confirmation := directedShowing("subject_request")
	confirmation["assessment_revision"] = old.AssessmentRevision
	var changed struct {
		Details struct {
			ReviewID string `json:"review_id"`
		} `json:"details"`
	}
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+oldID+"/direct-send", confirmation, nil, &changed); status != http.StatusConflict {
		t.Fatalf("stale confirmation=%d", status)
	}
	if changed.Details.ReviewID == "" || changed.Details.ReviewID == oldID {
		t.Fatal("no renewed review")
	}
	var current struct {
		AssessmentRevision string `json:"assessment_revision"`
	}
	if status := c.Call(t, "GET", "/v1/communication-reviews/"+changed.Details.ReviewID, nil, nil, &current); status != http.StatusOK {
		t.Fatalf("fresh review=%d", status)
	}
	if current.AssessmentRevision == old.AssessmentRevision {
		t.Fatal("a new stop reused the acknowledgement")
	}
	var instructions, deliveries int
	if err := c.Owner.QueryRow(t.Context(), `SELECT (SELECT count(*) FROM communication_instruction),(SELECT count(*) FROM comms_outbound)`).Scan(&instructions, &deliveries); err != nil {
		t.Fatal(err)
	}
	if instructions != 0 || deliveries != 0 {
		t.Fatal("stale acknowledgement recorded a decision or delivery")
	}
	confirmation["assessment_revision"] = current.AssessmentRevision
	if status := c.Call(t, "POST", "/v1/communication-reviews/"+changed.Details.ReviewID+"/direct-send", confirmation, nil, nil); status != http.StatusCreated {
		t.Fatalf("fresh confirmation=%d", status)
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/testdb"
)

func TestAnExpiredInstructionIsCheckedAgainBeforeItIsSpent(t *testing.T) {
	for _, tc := range []struct {
		name string
		stop bool
	}{{"basis still holds", false}, {"later subject stop", true}} {
		t.Run(tc.name, func(t *testing.T) {
			c := setupConsent(t)
			c.useReviewableMarketingProfile(t)
			if status := c.Call(t, "DELETE", "/v1/activities/"+c.activityID, nil, nil, nil); status != http.StatusOK {
				t.Fatalf("archiving the fixture's prior inbound = %d, want 200", status)
			}
			var anchor struct {
				ID string `json:"id"`
			}
			if status := c.Call(t, "POST", "/v1/activities", AnyMap{
				"kind": "email", "subject": "Follow-up draft", "direction": "outbound",
				"links": []AnyMap{{"entity_type": "contact", "entity_id": c.contactID}},
			}, nil, &anchor); status != http.StatusCreated {
				t.Fatalf("creating the outbound anchor = %d, want 201", status)
			}
			c.activityID = anchor.ID
			if status := c.Call(t, "POST", "/v1/activities/"+c.activityID+"/send-email", AnyMap{
				"subject": "Requested information", "body": "The requested information.",
				"to": []string{"subject@consent.test"}, "communication_context": "requested_followup",
			}, nil, nil); status != http.StatusConflict {
				t.Fatalf("initial send = %d, want 409", status)
			}
			review := liveReviewID(t, c)
			if _, err := c.Owner.Exec(t.Context(), `
				CREATE FUNCTION reject_exception_staging() RETURNS trigger LANGUAGE plpgsql AS $$
				BEGIN RAISE EXCEPTION 'delivery staging interrupted by the test'; END $$`); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if _, err := c.Owner.Exec(context.Background(), `DROP FUNCTION IF EXISTS reject_exception_staging() CASCADE`); err != nil {
					t.Error(err)
				}
			})
			if _, err := c.Owner.Exec(t.Context(), `
				CREATE TRIGGER reject_exception_staging BEFORE UPDATE ON communication_instruction
				FOR EACH ROW WHEN (NEW.status = 'consumed') EXECUTE FUNCTION reject_exception_staging()`); err != nil {
				t.Fatal(err)
			}
			path := "/v1/communication-reviews/" + review + "/direct-send"
			if status := c.Call(t, "POST", path, directed(), nil, nil); status != http.StatusInternalServerError {
				t.Fatalf("interrupted staging = %d, want 500", status)
			}
			if _, err := c.Owner.Exec(t.Context(), `DROP FUNCTION reject_exception_staging() CASCADE`); err != nil {
				t.Fatal(err)
			}
			// The real writer commits the decision before staging. Only its clock
			// fields are aged to exercise retry beyond the original window.
			testdb.WithFixtureTriggerDisabled(t, "communication_instruction", "communication_instruction_refuse_rewrite", func(tx pgx.Tx) error {
				_, err := tx.Exec(t.Context(), `
					UPDATE communication_instruction
					SET facts_as_of='2000-01-01T00:00:00Z', acknowledged_at='2000-01-01T00:00:00Z',
					    directed_at='2000-01-01T00:00:00Z', valid_until='2000-01-02T00:00:00Z'
					WHERE review_id=@review`, pgx.StrictNamedArgs{"review": review})
				return err
			})
			const frozen = `SELECT (to_jsonb(i)-ARRAY['status','consumed_at','delivery_id'])::text
				FROM communication_instruction i WHERE review_id=@review`
			var before, after string
			if err := c.Owner.QueryRow(t.Context(), frozen, pgx.StrictNamedArgs{"review": review}).Scan(&before); err != nil {
				t.Fatal(err)
			}
			if tc.stop {
				if status := c.Call(t, "POST", "/v1/contacts/"+c.contactID+"/consent/suppress", AnyMap{
					"kind": "subject_request", "reason": "Asked to stop before the retry.",
				}, nil, nil); status != http.StatusNoContent {
					t.Fatalf("recording the stop = %d, want 204", status)
				}
			}
			status := c.Call(t, "POST", path, directed(), nil, nil)
			if tc.stop {
				if status != http.StatusForbidden && status != http.StatusConflict && status != http.StatusUnprocessableEntity {
					t.Fatalf("retry after a stop = %d, want 403, 409 or 422", status)
				}
			} else if status != http.StatusCreated {
				t.Fatalf("retry with unchanged support = %d, want 201", status)
			}
			if err := c.Owner.QueryRow(t.Context(), frozen, pgx.StrictNamedArgs{"review": review}).Scan(&after); err != nil {
				t.Fatal(err)
			}
			if before != after {
				t.Fatal("retry rewrote the original author, scope, warning, message or dates")
			}
			want, wantStatus := 1, "consumed"
			if tc.stop {
				want, wantStatus = 0, "directed"
			} else if retry := c.Call(t, "POST", path, directed(), nil, nil); retry >= http.StatusInternalServerError {
				t.Fatalf("repeated retry = %d, want a completed or known refusal response", retry)
			}
			var deliveries int
			var instructionStatus string
			if err := c.Owner.QueryRow(t.Context(), `
				SELECT (SELECT count(*) FROM comms_outbound), status
				FROM communication_instruction WHERE review_id=@review`, pgx.StrictNamedArgs{"review": review}).Scan(&deliveries, &instructionStatus); err != nil {
				t.Fatal(err)
			}
			if deliveries != want || instructionStatus != wantStatus {
				t.Fatalf("deliveries = %d, instruction = %s; want %d, %s", deliveries, instructionStatus, want, wantStatus)
			}
		})
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A credential acts for its human with that human's permissions, so what the
// human could release in the CRM it may release from the conversation —
// including the change it proposed itself, when that change can be undone.

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestACredentialReleasesTheRecordChangeItProposedForItsHuman(t *testing.T) {
	q := setupQueue(t)
	contactID := createdID(t, q.AppEnv, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Selma Human"})
	invoke := q.invoker(t, q.mintPassport(t, "self-releasing agent", "read", "write"))

	out, err := invoke("update_record", fmt.Sprintf(
		`{"record_type":"contact","id":%q,"fields":{"full_name":"Selma Machine","title":"COO"}}`, contactID,
	))
	if err != nil {
		t.Fatalf("update_record → %v", err)
	}
	var split struct {
		StagedApproval struct {
			ApprovalID string          `json:"approval_id"`
			Replay     json.RawMessage `json:"replay"`
		} `json:"staged_approval"`
	}
	if err := json.Unmarshal(ToolPayload(t, json.RawMessage(out)), &split); err != nil {
		t.Fatal(err)
	}
	approvalID := split.StagedApproval.ApprovalID
	if approvalID == "" {
		t.Fatalf("overwriting the human's name staged nothing: %s", out)
	}

	if _, err := invoke("decide_approval", `{"staged_action_id":"`+approvalID+`","decision":"approve"}`); err != nil {
		t.Fatalf("the credential could not release the record change it proposed for its human: %v", err)
	}
	// The release is the human's, given through the agent: decided_by names the
	// lender and the audit row names the credential that carried it.
	var decidedByLender bool
	var auditActor string
	if err := q.Owner.QueryRow(t.Context(), `
		SELECT a.decided_by = p.on_behalf_of,
		       (SELECT l.actor_type FROM audit_log l
		         WHERE l.entity_id = a.id AND l.action = 'approve')
		  FROM approval a JOIN passport p ON p.id = a.passport_id
		 WHERE a.id = $1`, approvalID).Scan(&decidedByLender, &auditActor); err != nil {
		t.Fatalf("reading the decision back: %v", err)
	}
	if !decidedByLender || auditActor != "agent" {
		t.Errorf("decision recorded as lender=%v through actor %q, want the lender through the agent",
			decidedByLender, auditActor)
	}

	if _, err := invoke("update_record", replayWithApprovalID(t, split.StagedApproval.Replay, approvalID)); err != nil {
		t.Fatalf("redeeming the released change → %v", err)
	}
	var name string
	if err := q.Owner.QueryRow(t.Context(), `SELECT full_name FROM contact WHERE id = $1`, contactID).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "Selma Machine" {
		t.Errorf("full_name = %q after the released change was redeemed", name)
	}
}

// A message that has left cannot be taken back, so an effect that reaches
// outside the workspace stays the human's to release in the CRM. enrich is
// the one such confirm-first verb this composition stages without a mailbox.
func TestACredentialDoesNotReleaseTheOutboundCallItProposed(t *testing.T) {
	q := setupQueue(t)
	invoke := q.invoker(t, q.mintPassport(t, "outbound agent", "read", "write", "enrich"))
	_, approvalID := q.stageAConfirmFirstCall(t, invoke, "Outbound Subject")

	_, err := invoke("decide_approval", `{"staged_action_id":"`+approvalID.String()+`","decision":"approve"}`)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("the proposer releasing its own outbound call → %v, want ErrPermissionDenied", err)
	}
}

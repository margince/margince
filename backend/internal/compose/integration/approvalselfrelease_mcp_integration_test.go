// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A credential acts for its human with that human's permissions. What the human
// could do unaided it does at once, and what still waits stays theirs to release.

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// The colleague behind a credential may overwrite a value someone typed, so the
// credential does it at once. Nothing is staged for anyone to release.
func TestACredentialOverwritesATypedValueAtOnceAndStagesNothing(t *testing.T) {
	q := setupQueue(t)
	contactID := createdID(t, q.AppEnv, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Selma Human"})
	invoke := q.invoker(t, q.mintPassport(t, "overwriting agent", "read", "write"))

	out, err := invoke("update_record", fmt.Sprintf(
		`{"record_type":"contact","id":%q,"fields":{"full_name":"Selma Machine","title":"COO"}}`, contactID,
	))
	if err != nil {
		t.Fatalf("update_record → %v", err)
	}
	var split struct {
		StagedApproval *json.RawMessage `json:"staged_approval"`
	}
	if err := json.Unmarshal(ToolPayload(t, json.RawMessage(out)), &split); err != nil {
		t.Fatal(err)
	}
	if split.StagedApproval != nil {
		t.Errorf("overwriting a typed name on a passport staged an approval: %s", out)
	}
	var name string
	var staged int
	if err := q.Owner.QueryRow(t.Context(), `
		SELECT c.full_name, (SELECT count(*) FROM approval WHERE target_entity_id = c.id)
		  FROM contact c WHERE c.id = $1`, contactID).Scan(&name, &staged); err != nil {
		t.Fatal(err)
	}
	if name != "Selma Machine" || staged != 0 {
		t.Errorf("full_name = %q with %d approvals staged, want the overwrite applied and none staged", name, staged)
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

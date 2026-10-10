// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"fmt"
	"strings"
	"testing"
)

// A null in an update_record patch clears the field, as it does over REST.
// A field the record cannot clear is refused by name, never left as it was.
func TestUpdateRecordToolClearsOnANullAndRefusesWhatItCannotClear(t *testing.T) {
	e := setupRelationships(t)
	agentToken := mintAgentToken(t, e)
	invoke := mcpAgentInvoker(t, agentToken)

	// A field this agent wrote is not a human's, so the clear applies at once.
	if _, err := invoke("update_record", fmt.Sprintf(
		`{"record_type":"contact","id":%q,"fields":{"title":"CTO"}}`, e.contactID)); err != nil {
		t.Fatalf("setting the title: %v", err)
	}
	if _, err := invoke("update_record", fmt.Sprintf(
		`{"record_type":"contact","id":%q,"fields":{"title":null}}`, e.contactID)); err != nil {
		t.Fatalf("clearing the title with a null: %v", err)
	}
	if _, title := contactNameAndTitle(t, invoke, e.contactID); title != "" {
		t.Errorf("title = %q after a null, want it cleared", title)
	}

	// A field the contact cannot clear is refused, never answered "updated".
	_, err := invoke("update_record", fmt.Sprintf(
		`{"record_type":"contact","id":%q,"fields":{"full_name":null}}`, e.contactID))
	if err == nil || !strings.Contains(err.Error(), "full_name") {
		t.Errorf("a null full_name → %v, want a refusal naming full_name", err)
	}

	created, err := invoke("create_record", fmt.Sprintf(
		`{"record_type":"relationship","fields":{"kind":"employment","contact_id":%q,"company_id":%q,"source":"manual","role":"cto"}}`,
		e.contactID, e.companyID))
	if err != nil {
		t.Fatalf("creating the edge: %v", err)
	}
	edgeID, _ := wireEdge(t, created)
	cleared, err := invoke("update_record", fmt.Sprintf(
		`{"record_type":"relationship","id":%q,"fields":{"role":null}}`, edgeID))
	if err != nil {
		t.Fatalf("clearing the role with a null: %v", err)
	}
	if _, edge := wireEdge(t, cleared); edge.Role != nil {
		t.Errorf("role = %q after a null, want it cleared", *edge.Role)
	}
}

// mintAgentToken issues a write passport for the signed-in workspace.
func mintAgentToken(t *testing.T, e *relEnv) string {
	t.Helper()
	var minted struct {
		Token string `json:"token"`
	}
	if status := e.Call(t, "POST", "/v1/passports", AnyMap{
		"label": "null clears", "scopes": []string{"read", "write"},
	}, nil, &minted); status != 201 {
		t.Fatalf("issue passport → %d", status)
	}
	return minted.Token
}

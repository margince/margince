// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

// The second audit door, judged by what reaches the column. An occurrence-shaped
// write must land the same SQL NULL a hand-written nil landed, or every
// "there was no prior state" query in the tree reads it differently from the
// rows it already holds.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// auditingContext binds the actor every audit write derives its identity from.
// A missing one is a programming error the writer refuses before any SQL, so a
// test without it would prove nothing about the images.
func auditingContext() context.Context {
	ctx := principal.WithActor(context.Background(),
		principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String()})
	return principal.WithWorkspaceID(ctx, ids.NewV7())
}

// Each image's POSITION in the INSERT's argument list, so the assertions below
// read a column rather than an index. The list starts at the row id, so these
// trail the placeholder numbers in the statement by one.
const (
	auditBeforeArg   = 8
	auditAfterArg    = 9
	auditEvidenceArg = 10
)

func TestAuditEventLandsSQLNullInTheBeforeColumn(t *testing.T) {
	tx := &fakeTx{}
	if _, err := AuditEvent(auditingContext(), tx, "update", "webhook_subscription", ids.NewV7(),
		map[string]any{"signing_secret_rotated": true}); err != nil {
		t.Fatalf("AuditEvent: %v", err)
	}
	// AbsentImage, not `== nil`: marshalOrNil answers nil BYTES, and a
	// []byte(nil) inside an any is a non-nil interface. That is the same
	// typed-nil trap this door exists to keep out of the column, so the
	// assertion has to ask the question the way the writer does.
	if got := tx.execArgs[auditBeforeArg]; !AbsentImage(got) {
		t.Errorf("before column got %v, want an absent image so it stores SQL NULL", got)
	}
	if AbsentImage(tx.execArgs[auditAfterArg]) {
		t.Error("the after image was dropped; an occurrence still records what happened")
	}
}

// The door exists so a reader can tell "there was nothing" from "nobody looked".
// That claim is only worth anything if the two spellings reach the column
// identically — otherwise converting a site would change what the row says.
func TestAuditEventAndAHandWrittenNilBeforeReachTheColumnAlike(t *testing.T) {
	after := map[string]any{"replayed_delivery": "d-1"}
	id := ids.NewV7()

	viaEvent := &fakeTx{}
	if _, err := AuditEvent(auditingContext(), viaEvent, "update", "webhook_subscription", id, after); err != nil {
		t.Fatalf("AuditEvent: %v", err)
	}
	viaAudit := &fakeTx{}
	if _, err := writeAuditRow(auditingContext(), viaAudit, "update", "webhook_subscription", id, nil, after, nil); err != nil {
		t.Fatalf("writeAuditRow: %v", err)
	}

	if viaEvent.execSQL != viaAudit.execSQL {
		t.Error("the two doors send different statements")
	}
	if string(viaEvent.execArgs[auditAfterArg].([]byte)) != string(viaAudit.execArgs[auditAfterArg].([]byte)) {
		t.Errorf("after images differ: %s vs %s", viaEvent.execArgs[auditAfterArg], viaAudit.execArgs[auditAfterArg])
	}
}

// Evidence is where context ABOUT a mutation goes. An occurrence-shaped write
// needs it as much as a field-shaped one — more, since it has no images to
// carry meaning — so the door must not be the reason a writer folds operation
// metadata into the after image instead.
func TestAuditEventWithEvidenceKeepsTheImagesAndTheEvidenceApart(t *testing.T) {
	tx := &fakeTx{}
	if _, err := AuditEventWithEvidence(auditingContext(), tx, "update", "provider_connection", ids.NewV7(),
		map[string]any{"status": "degraded"}, map[string]any{"safe_status_code": 503}); err != nil {
		t.Fatalf("AuditEventWithEvidence: %v", err)
	}
	if got := tx.execArgs[auditBeforeArg]; !AbsentImage(got) {
		t.Errorf("before column got %v, want SQL NULL", got)
	}
	if AbsentImage(tx.execArgs[auditAfterArg]) {
		t.Error("the after image was dropped")
	}
	if AbsentImage(tx.execArgs[auditEvidenceArg]) {
		t.Error("the evidence was dropped")
	}
}

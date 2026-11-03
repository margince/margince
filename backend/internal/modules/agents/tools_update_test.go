// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// update_record's own specs. The egress governance that used to live here
// moved to the datasource seam (compose/dispatcher.go): the tool is not the
// only route to an incumbent write, so a per-tool gate could not be the
// control. What remains here is the tool's own contract.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func contactFixture(t *testing.T, id ids.UUID) *fakeSoR {
	t.Helper()
	ref := datasource.EntityRef{Type: datasource.EntityContact, ID: id}
	return &fakeSoR{records: map[datasource.EntityRef]datasource.Record{
		ref: nativeRecord(datasource.Record{
			Ref: ref, Fields: json.RawMessage(`{"full_name":"Ada Lovelace"}`), Version: 3,
		}),
	}}
}

func updateArgs(t *testing.T, id ids.UUID) json.RawMessage {
	t.Helper()
	in, err := json.Marshal(map[string]any{
		"record_type": "contact",
		"id":          id,
		"fields":      map[string]any{"title": "Analyst"},
	})
	if err != nil {
		t.Fatalf("marshaling args: %v", err)
	}
	return in
}

// An agent's patch applies in the one call, whoever last wrote the fields.
func TestUpdateRecordAppliesTheWholePatch(t *testing.T) {
	id := ids.NewV7()
	p := contactFixture(t, id)
	tool := updateRecord{p: p}

	if _, err := tool.Handle(context.Background(), updateArgs(t, id)); err != nil {
		t.Fatalf("Handle err = %v, want nil", err)
	}
	if len(p.updates) != 1 {
		t.Errorf("provider.Update called %d time(s), want 1", len(p.updates))
	}
}

// A released call performs the effect the human approved.
func TestUpdateRecordAppliesAReleasedCall(t *testing.T) {
	id := ids.NewV7()
	p := contactFixture(t, id)
	tool := updateRecord{p: p}

	ctx := withApprovalRedeemed(context.Background(), 0, false)
	if _, err := tool.Handle(ctx, updateArgs(t, id)); err != nil {
		t.Fatalf("Handle err = %v, want nil", err)
	}
	if len(p.updates) != 1 {
		t.Errorf("provider.Update called %d time(s) after release, want 1", len(p.updates))
	}
}

// A 🟡 tool must not mint an approval for a record whose authority lives
// elsewhere: the human would be asked to release a change that redemption can
// never verify, because the pin and the decidability probe both read our own
// tables. Refusing keeps every staging tool agreeing with the datasource seam,
// which refuses the same write for the same reason.
func TestStagingIsRefusedForANonAuthoritativeTarget(t *testing.T) {
	ref := datasource.EntityRef{Type: datasource.EntityContact, ID: ids.NewV7()}
	mirrored := datasource.Record{Ref: ref, Fields: json.RawMessage(`{}`)}

	err := refuseStagingElsewhere(mirrored)

	if !errors.Is(err, apperrors.ErrUnsupportedBySoR) {
		t.Fatalf("err = %v, want ErrUnsupportedBySoR", err)
	}
	if err := refuseStagingElsewhere(nativeRecord(mirrored)); err != nil {
		t.Errorf("a locally-authoritative record must stage: %v", err)
	}
}

// update_record stages through StageInfo when the contract floor tightens it.
// For a target held elsewhere it must refuse rather than mint an approval no
// human can release.
func TestUpdateRecordRefusesStagingForATargetHeldElsewhere(t *testing.T) {
	id := ids.NewV7()
	ref := datasource.EntityRef{Type: datasource.EntityContact, ID: id}
	// Deliberately NOT nativeRecord: this record's authority lives elsewhere.
	p := &fakeSoR{records: map[datasource.EntityRef]datasource.Record{
		ref: {Ref: ref, Fields: json.RawMessage(`{"title":"Analyst"}`), Version: 3},
	}}
	tool := updateRecord{p: p}

	_, err := tool.StageInfo(context.Background(), updateArgs(t, id))

	if !errors.Is(err, apperrors.ErrUnsupportedBySoR) {
		t.Fatalf("StageInfo err = %v, want ErrUnsupportedBySoR", err)
	}
	if len(p.updates) != 0 {
		t.Errorf("provider.Update called %d time(s), want 0", len(p.updates))
	}
}

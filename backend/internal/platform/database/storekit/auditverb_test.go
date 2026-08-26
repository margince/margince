// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import "testing"

// The zero value is `update`, so every existing caller keeps its verb without
// naming it. A restore names its own.
func TestAuditVerbZeroValueIsUpdate(t *testing.T) {
	var v AuditVerb
	if got := v.OrUpdate(); got != VerbUpdate {
		t.Fatalf("zero AuditVerb = %q, want %q", got, VerbUpdate)
	}
	if got := VerbRestore.OrUpdate(); got != VerbRestore {
		t.Fatalf("VerbRestore.OrUpdate() = %q, want %q", got, VerbRestore)
	}
}

// Only verbs the audit CHECK admits may be written through the update door.
// A typo must fail here rather than at the database.
func TestAuditVerbRejectsUnknown(t *testing.T) {
	if AuditVerb("updat").Valid() {
		t.Fatal("AuditVerb(\"updat\").Valid() = true, want false")
	}
	if AuditVerb("archive").Valid() {
		t.Fatal("AuditVerb(\"archive\").Valid() = true, want false: " +
			"the update door admits two verbs, not every verb the CHECK takes")
	}
}

// An unknown verb is refused at the door, so the failure names the mistake
// rather than an audit_log CHECK constraint.
func TestAuditTrailRefusesAVerbTheDoorMayNotWrite(t *testing.T) {
	if _, _, err := (AuditTrail{Verb: AuditVerb("archive")}).Resolve(); err == nil {
		t.Fatal("Resolve accepted \"archive\"; the update door admits two verbs")
	}
}

// The zero Trail is an ordinary update carrying no evidence, so a caller that
// never heard of the reversal path keeps exactly the write it had.
func TestZeroAuditTrailIsAnOrdinaryUpdate(t *testing.T) {
	action, evidence, err := AuditTrail{}.Resolve()
	if err != nil {
		t.Fatalf("the zero Trail was refused: %v", err)
	}
	if action != string(VerbUpdate) {
		t.Errorf("zero Trail action = %q, want %q", action, VerbUpdate)
	}
	if evidence != nil {
		t.Errorf("zero Trail evidence = %v, want nil", evidence)
	}
}

// A restore names the row it reverses; the evidence rides through unchanged.
func TestAuditTrailCarriesTheEvidenceThrough(t *testing.T) {
	want := map[string]any{"undid_audit_log_id": "row"}
	action, evidence, err := (AuditTrail{Verb: VerbRestore, Evidence: want}).Resolve()
	if err != nil {
		t.Fatalf("a restore Trail was refused: %v", err)
	}
	if action != string(VerbRestore) {
		t.Errorf("action = %q, want %q", action, VerbRestore)
	}
	if evidence["undid_audit_log_id"] != "row" {
		t.Errorf("evidence did not ride through: %v", evidence)
	}
}

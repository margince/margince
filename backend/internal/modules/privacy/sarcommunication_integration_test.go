// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

// Whether the outbound section names the ONE purpose a stop was narrowed to.
//
// communication_suppression.purpose_id lets a stop bind a single marketing
// purpose rather than all of them (see the consent module's suppressionBinds).
// Art. 15 owes the subject what is HELD, and a stop scoped to one purpose is a
// narrower fact than "they objected to everything" — an export that dropped
// the column would tell a subject holding a per-purpose stop that the
// installation recorded a broader objection than it did.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// TestTheExportNamesThePurposeAPerPurposeStopWasNarrowedTo.
func TestTheExportNamesThePurposeAPerPurposeStopWasNarrowedTo(t *testing.T) {
	e := setupSARIdentifiers(t)

	var purposeID ids.UUID
	if err := e.owner.QueryRow(e.ctx, `
		INSERT INTO consent_purpose (key, label, class)
		VALUES ('newsletter', 'Newsletter', 'marketing')
		RETURNING id`).Scan(&purposeID); err != nil {
		t.Fatalf("seeding the marketing purpose: %v", err)
	}
	if _, err := e.owner.Exec(e.ctx, `
		INSERT INTO communication_suppression (contact_id, kind, source, captured_by, decided_by_level, purpose_id)
		VALUES ($1, 'marketing_objection', 'operator_ui', 'human:x', 'subject', $2)`,
		e.contact, purposeID); err != nil {
		t.Fatalf("seeding the per-purpose stop: %v", err)
	}

	pkg, err := AssembleSAR(e.ctx, e.db, e.contact)
	if err != nil {
		t.Fatalf("AssembleSAR: %v", err)
	}

	if len(pkg.CommunicationSuppression) == 0 {
		t.Fatal("the export carries no stop at all, though one was seeded")
	}
	// BY NAME, because the subject is the reader. An id would satisfy "the
	// column is exported" and still leave them unable to tell which list they
	// left, which is the whole reason this section names a purpose.
	got, ok := pkg.CommunicationSuppression[0]["purpose_key"]
	if !ok {
		t.Fatal("the export's suppression row carries no purpose at all — a subject holding a " +
			"per-purpose stop cannot tell it apart from one that objects to everything")
	}
	if got != "newsletter" {
		t.Errorf("the export's suppression row names purpose %v, want the key \"newsletter\" — "+
			"a subject cannot read a consent_purpose id", got)
	}
	if label := pkg.CommunicationSuppression[0]["purpose_label"]; label != "Newsletter" {
		t.Errorf("the export's suppression row labels the purpose %v, want %q", label, "Newsletter")
	}
}

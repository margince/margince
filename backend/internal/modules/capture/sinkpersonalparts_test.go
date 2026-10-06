// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// What a private thread's files leave behind.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The whole point: a private thread's bytes never reach anything that stores
// them, and the count says how many were kept out.
func TestAPrivateThreadsFilesAreTakenOutBeforeAnythingStoresThem(t *testing.T) {
	rec := connector.NormalizedRecord{Parts: []connector.Part{
		{Ordinal: 1, Filename: "gehaltsabrechnung.pdf", Body: []byte("payslip")},
		{Ordinal: 2, Filename: "attest.pdf", Body: []byte("medical")},
	}}

	stripped, taken := stripPersonalParts(rec)
	if len(taken) != 2 {
		t.Fatalf("took %d parts, want both files, so both can still be named", len(taken))
	}
	if len(stripped.Parts) != 0 {
		t.Fatalf("parts = %d, want none left to stage", len(stripped.Parts))
	}
}

// Removed rather than emptied. Staging writes every body unconditionally, so
// an emptied part would put a zero-length object in the store under a key the
// message then points at — storing something, which is worse than storing
// either the file or nothing.
func TestAWithheldFileLeavesNoEmptyPartToStage(t *testing.T) {
	rec := connector.NormalizedRecord{Parts: []connector.Part{
		{Ordinal: 1, Filename: "schulformular.pdf", Body: []byte("form")},
	}}

	stripped, _ := stripPersonalParts(rec)
	for _, part := range stripped.Parts {
		t.Fatalf("a part survived with body length %d — staging would write it", len(part.Body))
	}
}

// A message that carried nothing withholds nothing, so no breadcrumb claims it
// did. A log saying "0 files withheld" on every ordinary message is noise that
// makes the real ones harder to find.
func TestAMessageWithNoFilesWithholdsNothing(t *testing.T) {
	_, taken := stripPersonalParts(connector.NormalizedRecord{})
	if len(taken) != 0 {
		t.Fatalf("took %d parts from a message that carried nothing", len(taken))
	}
}

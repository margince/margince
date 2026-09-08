// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

// Pure unit coverage for the workflow_run.detail jsonb shape (migration
// 0077): no database needed, since every writer and reader here is a
// plain encode/decode pair over the same runDetail struct.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestReasonDetailRoundTripsThroughDecodeRunDetail(t *testing.T) {
	payload, err := reasonDetail("provider timeout")
	if err != nil {
		t.Fatalf("reasonDetail: %v", err)
	}
	reason, err := decodeRunDetail(payload)
	if err != nil {
		t.Fatalf("decodeRunDetail: %v", err)
	}
	if reason == nil || *reason != "provider timeout" {
		t.Fatalf("decodeRunDetail(reasonDetail(%q)) = %v, want the same reason back", "provider timeout", reason)
	}
	parsed, err := parseRunDetail(payload)
	if err != nil {
		t.Fatalf("parseRunDetail: %v", err)
	}
	if parsed.ApprovalID != nil {
		t.Fatalf("a plain reason carries approval_id = %v, want nil (no staging pointer)", parsed.ApprovalID)
	}
}

// TestStagedApprovalDetailRoundTripsTheApprovalID pins the fix this
// migration exists for: the staging pointer must survive round-trip as
// a PARSED field, not a substring a caller has to re-derive by matching
// the whole sentence.
func TestStagedApprovalDetailRoundTripsTheApprovalID(t *testing.T) {
	approvalID := ids.New[ids.ApprovalKind]()
	payload, err := stagedApprovalDetail(approvalID)
	if err != nil {
		t.Fatalf("stagedApprovalDetail: %v", err)
	}
	parsed, err := parseRunDetail(payload)
	if err != nil {
		t.Fatalf("parseRunDetail: %v", err)
	}
	if parsed.ApprovalID == nil || *parsed.ApprovalID != approvalID {
		t.Fatalf("parsed approval_id = %v, want %s", parsed.ApprovalID, approvalID)
	}
	if !strings.Contains(parsed.Reason, approvalID.String()) {
		t.Fatalf("reason %q does not name the staged approval, want a human-readable mention too", parsed.Reason)
	}
	// The reason still decodes through the same reader the run-history
	// API uses, for the human display path.
	reason, err := decodeRunDetail(payload)
	if err != nil {
		t.Fatalf("decodeRunDetail: %v", err)
	}
	if reason == nil || !strings.Contains(*reason, approvalID.String()) {
		t.Fatalf("decoded reason = %v, want it to name the approval", reason)
	}
}

// TestParseRunDetailHandlesTheNullRun is the honest empty case: a clean
// applied run stores no detail at all, which Postgres hands back as a
// nil (or zero-length) scan — that must parse to the zero value, not an
// error, so ListRuns never chokes on the common case.
func TestParseRunDetailHandlesTheNullRun(t *testing.T) {
	parsed, err := parseRunDetail(nil)
	if err != nil {
		t.Fatalf("parseRunDetail(nil): %v", err)
	}
	if parsed != (runDetail{}) {
		t.Fatalf("parseRunDetail(nil) = %+v, want the zero value", parsed)
	}
	reason, err := decodeRunDetail(nil)
	if err != nil {
		t.Fatalf("decodeRunDetail(nil): %v", err)
	}
	if reason != nil {
		t.Fatalf("decodeRunDetail(nil) = %v, want nil (no reason to render)", reason)
	}
}

// TestParseRunDetailSurfacesAMalformedPayload is the T7 hard case this
// column change must not regress: a detail column that fails to
// unmarshal is a defect in the stored data, and must be reported as a
// read error — never silently read back as an empty/absent reason.
func TestParseRunDetailSurfacesAMalformedPayload(t *testing.T) {
	if _, err := parseRunDetail([]byte("not json")); err == nil {
		t.Fatal("parseRunDetail accepted malformed jsonb without error")
	}
	if _, err := decodeRunDetail([]byte("not json")); err == nil {
		t.Fatal("decodeRunDetail accepted malformed jsonb without error")
	}
}

// A decline's reason reaches workflow_run.detail, which anybody holding
// automation:read reads verbatim. workflow.Declined takes a plain string from a
// module, so this is the one place standing between a handler and that reader.
func TestADeclinedReasonKeepsItsSentenceAndRefusesADatabaseInternal(t *testing.T) {
	const generic = "the automation found nothing to do"

	kept := "no eligible owner had capacity for this lead"
	if got := declinedReason(kept); got != kept {
		t.Errorf("a written reason came back %q, want it kept — collapsing it "+
			"would erase the only thing a decline exists to say", got)
	}

	// Each of these is a message that came from somewhere other than an
	// author, and each is a way a backend internal reaches a client.
	for _, leak := range []string{
		`ERROR: duplicate key value violates unique constraint "lead_pkey" (SQLSTATE 23505)`,
		`pq: relation "workflow_run" does not exist`,
		`pgx: column "owner_id" cannot be null`,
		`a reason naming a "quoted identifier"`,
		strings.Repeat("prose ", 40),
		"",
		"   ",
	} {
		if got := declinedReason(leak); got != generic {
			t.Errorf("declinedReason(%.40q…) = %q, want the generic phrase — a "+
				"reader holding automation:read must not meet a database internal", leak, got)
		}
	}
}

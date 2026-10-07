// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

// The headers-first question gives Upsert's own answer. A backfill asks
// DropBeforeStore before downloading a message in full, so a verdict that
// differed from Upsert's would either drop customer mail or download colleague
// mail for nothing.

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture"
)

func TestDropBeforeStoreAgreesWithUpsert(t *testing.T) {
	ctx, db := bootstrapInternalMailWorkspace(t, "acme.com")
	sink := capture.NewSink(db)

	internal := mailRecord("prestore-internal", "boss@acme.com", "boss@acme.com", "rep@acme.com")
	internal.Raw = nil
	drop, err := sink.DropBeforeStore(ctx, internal)
	if err != nil || !drop {
		t.Fatalf("all-internal headers: drop=%v err=%v, want dropped", drop, err)
	}
	// The drop leaves the breadcrumb Upsert's drop leaves, and nothing else.
	if reasons := breadcrumbReasons(ctx, t, db.Pool(), "prestore-internal"); len(reasons) != 1 || reasons[0] != "internal_only" {
		t.Errorf("ledger reasons = %v, want exactly one internal_only", reasons)
	}
	if activities, _, raws, _ := countsFor(ctx, t, db.Pool(), "prestore-internal"); activities != 0 || raws != 0 {
		t.Errorf("the pre-store question stored rows: activity=%d raw_capture=%d", activities, raws)
	}

	external := mailRecord("prestore-external", "buyer@customer.example", "buyer@customer.example", "rep@acme.com")
	external.Raw = nil
	if drop, err := sink.DropBeforeStore(ctx, external); err != nil || drop {
		t.Fatalf("customer headers: drop=%v err=%v, want kept for the full download", drop, err)
	}
	if activities, _, _, _ := countsFor(ctx, t, db.Pool(), "prestore-external"); activities != 0 {
		t.Errorf("the pre-store question stored the kept message; only Upsert may")
	}
}

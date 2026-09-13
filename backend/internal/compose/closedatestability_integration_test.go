// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
)

func TestCloseDateNearFutureCommitmentDoesNotSlideOnConsecutiveNights(t *testing.T) {
	e := setupCloseDate(t)
	id := e.seedSweepDeal(t, "Stable estimate", e.early, nil, intp(5), 3)
	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	first := e.readSwept(t, id)
	if first.expectedClose == nil || first.provisional || !first.expectedClose.Equal(today().AddDate(0, 0, 5)) {
		t.Fatal("valid near-future date was replaced with an estimate")
	}
	var before int
	if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE entity_id = $1`, id).Scan(&before); err != nil {
		t.Fatal(err)
	}
	e.day = e.day.AddDate(0, 0, 1)
	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}
	second := e.readSwept(t, id)
	if second.expectedClose == nil || !first.expectedClose.Equal(*second.expectedClose) {
		t.Fatalf("date slid: %v -> %v", first.expectedClose, second.expectedClose)
	}
	var after int
	if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE entity_id = $1`, id).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("unchanged estimate wrote another receipt: audits %d -> %d", before, after)
	}
}

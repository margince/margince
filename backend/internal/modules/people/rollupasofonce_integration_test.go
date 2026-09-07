// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// One company read, one as-of date.
//
// The read asks the rollup twice — once for the open-deal count on the row,
// once for the pipeline total in the computed fields — and both take a date to
// convert at. Sampled separately they can straddle UTC midnight, and the page
// then shows a count from one day beside a total from the next: the same
// unreproducible disagreement binding the date was meant to end, one level
// further down.
//
// The clock advances a full day per call here, so a second sample cannot be
// mistaken for the first.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestOneCompanyReadBindsOneAsOfDate(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	org, err := e.store.CreateOrganization(ctx, CreateOrganizationInput{
		DisplayName: "Midnight Glazing GmbH", Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}

	original := rollupClock
	t.Cleanup(func() { rollupClock = original })
	day := time.Date(2026, 6, 2, 23, 59, 0, 0, time.UTC)
	var samples []time.Time
	rollupClock = func() time.Time {
		samples = append(samples, day)
		day = day.AddDate(0, 0, 1)
		return samples[len(samples)-1]
	}

	if _, err := e.store.GetOrganization(ctx,
		ids.From[ids.OrganizationKind](ids.UUID(org.Id)), storekit.LiveOnly); err != nil {
		t.Fatal(err)
	}

	if len(samples) != 1 {
		t.Fatalf("the read sampled the clock %d times, want once — the count and the pipeline "+
			"total would be asked for different days whenever midnight fell between them", len(samples))
	}
}

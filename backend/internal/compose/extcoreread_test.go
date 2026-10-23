// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/ratelimit"
	"github.com/margince/margince/backend/pkg/extension"
)

// everyReadVerb calls each read verb once on core.
func everyReadVerb(ctx context.Context, core extension.Core) map[string]error {
	const id = "00000000-0000-0000-0000-000000000001"
	out := map[string]error{}
	_, out["deals.get"] = core.Deals().Get(ctx, id)
	_, out["deals.list"] = core.Deals().List(ctx, extension.DealQuery{})
	_, out["companies.get"] = core.Companies().Get(ctx, id)
	_, out["companies.list"] = core.Companies().List(ctx, extension.CompanyQuery{})
	_, out["signals.get"] = core.Signals().Get(ctx, id)
	_, out["signals.list"] = core.Signals().List(ctx, extension.SignalQuery{})
	_, out["activities.get"] = core.Activities().Get(ctx, id)
	_, out["activities.list"] = core.Activities().List(ctx, extension.ActivityQuery{})
	_, out["customfields.list"] = core.CustomFields().List(ctx, "deal")
	return out
}

func TestAJobTickAndABusDeliveryAreRefusedEveryCoreRead(t *testing.T) {
	core := extensionCore{unattended: true, unit: "tick-unit"}
	for verb, err := range everyReadVerb(context.Background(), core) {
		if !errors.Is(err, extension.ErrForbidden) {
			t.Errorf("%s on a tick answered %v, want ErrForbidden", verb, err)
		} else if !strings.Contains(err.Error(), "core read or write") {
			t.Errorf("%s on a tick said %q, want a refusal that names reads as well as writes", verb, err)
		}
	}
}

func attendedCore(unit string) extensionCore {
	return extensionCore{
		unit:      unit,
		authority: func(ctx context.Context) (context.Context, error) { return ctx, nil },
	}
}

func TestAUnitIsHeldToItsReadsPerWindowAndOtherUnitsAreNot(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	was := extensionReads
	extensionReads = ratelimit.NewWithClock("test-extension-core-read", ratelimit.FailOpen, 2, time.Minute, func() time.Time { return now })
	t.Cleanup(func() { extensionReads = was })

	busy := attendedCore("busy").CustomFields()
	// An unknown object is refused as invalid after the read is metered, so
	// this test spends reads without a database.
	for i := range 2 {
		if _, err := busy.List(context.Background(), "nope"); !errors.Is(err, extension.ErrInvalid) {
			t.Fatalf("read %d answered %v, want ErrInvalid", i+1, err)
		}
	}
	if _, err := busy.List(context.Background(), "nope"); !errors.Is(err, extension.ErrRateLimited) {
		t.Errorf("a read over the cap answered %v, want ErrRateLimited", err)
	}
	if _, err := attendedCore("quiet").CustomFields().List(context.Background(), "nope"); !errors.Is(err, extension.ErrInvalid) {
		t.Errorf("another unit's read answered %v, want it unaffected by the busy unit", err)
	}
	now = now.Add(time.Minute)
	if _, err := busy.List(context.Background(), "nope"); !errors.Is(err, extension.ErrInvalid) {
		t.Errorf("a read in the next window answered %v, want it admitted again", err)
	}
}

func TestAListOnAPortWithNoAuditQueueIsRefusedRatherThanLeftUnrecorded(t *testing.T) {
	if err := (extensionCore{unit: "unit"}).auditBulkRead("deal", 3, nil); !errors.Is(err, errNoReadAudit) {
		t.Errorf("auditBulkRead answered %v, want errNoReadAudit", err)
	}
}

func TestAPageLimitIsCutToTheCeilingAndANegativeOneIsRefused(t *testing.T) {
	if got, err := readLimit(0); got != nil || err != nil {
		t.Errorf("readLimit(0) = %v, %v; want the store's default", got, err)
	}
	if got, err := readLimit(extension.MaxReadPage * 5); err != nil || *got != extension.MaxReadPage {
		t.Errorf("readLimit(%d) = %v, %v; want %d", extension.MaxReadPage*5, got, err, extension.MaxReadPage)
	}
	if _, err := readLimit(-1); !errors.Is(err, extension.ErrInvalid) {
		t.Errorf("readLimit(-1) = %v, want ErrInvalid", err)
	}
}

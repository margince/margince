// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/ratelimit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

func TestTheReadHelpersTreatEmptyAsNoFilterAndRefuseWhatIsMalformed(t *testing.T) {
	if pageLimit(0) != nil || *pageLimit(7) != 7 {
		t.Error("pageLimit: zero must be nil and a count must be kept")
	}
	if optional("") != nil || *optional("x") != "x" {
		t.Error("optional: empty must be nil and a value must be kept")
	}
	if got, err := idFilter[ids.DealKind](""); got != nil || err != nil {
		t.Errorf("idFilter(empty) = %v, %v; want no filter", got, err)
	}
	if got, err := idFilter[ids.DealKind]("00000000-0000-0000-0000-000000000001"); got == nil || err != nil {
		t.Errorf("idFilter(uuid) = %v, %v; want a parsed id", got, err)
	}
	if _, err := idFilter[ids.DealKind]("nope"); !errors.Is(err, extension.ErrInvalid) {
		t.Errorf("idFilter(nope) = %v, want ErrInvalid", err)
	}
	if err := bothOrNeither("", "", "a pair"); err != nil {
		t.Errorf("bothOrNeither(neither) = %v", err)
	}
	if err := bothOrNeither("a", "b", "a pair"); err != nil {
		t.Errorf("bothOrNeither(both) = %v", err)
	}
	if err := bothOrNeither("a", "", "a pair"); !errors.Is(err, extension.ErrInvalid) {
		t.Errorf("bothOrNeither(half) = %v, want ErrInvalid", err)
	}
}

func TestTheAuditNamesOnlyTheFiltersUsed(t *testing.T) {
	got := filterNames(map[string]string{"cf_region": "x"}, usedFilter{"search", "q"}, usedFilter{"status", ""})
	if len(got) != 2 || got[0] != "search" || got[1] != "custom_fields" {
		t.Errorf("filterNames = %v, want [search custom_fields]", got)
	}
	if got := filterNames(nil, usedFilter{"search", ""}); len(got) != 0 {
		t.Errorf("filterNames with nothing used = %v", got)
	}
}

func TestADealQueryBecomesTheStoresInputAndABadOneIsRefused(t *testing.T) {
	const id = "00000000-0000-0000-0000-000000000001"
	in, err := dealInput(extension.DealQuery{Search: "fleet", StageID: id, OwnerID: id, CompanyID: id, SourceSystem: "erp", SourceID: "9"})
	if err != nil || in.StageID == nil || in.OwnerID == nil || in.CompanyID == nil || *in.Query != "fleet" || *in.SourceID != "9" {
		t.Fatalf("dealInput = %+v, %v", in, err)
	}
	for name, q := range map[string]extension.DealQuery{
		"negative limit": {ReadPage: extension.ReadPage{Limit: -1}},
		"half a pair":    {SourceSystem: "erp"},
		"bad stage":      {StageID: "x"},
		"bad owner":      {OwnerID: "x"},
		"bad company":    {CompanyID: "x"},
	} {
		if _, err := dealInput(q); !errors.Is(err, extension.ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
}

func TestAPublishedPageIsNeverNilAndAShapeMismatchIsRefused(t *testing.T) {
	page, err := publish[map[string]any]([]map[string]any(nil), storekit.Page{NextCursor: "c"})
	if err != nil || page.Items == nil || page.NextCursor != "c" {
		t.Errorf("publish(empty) = %+v, %v; want a non-nil empty page", page, err)
	}
	if _, err := publish[int]([]string{"not a number"}, storekit.Page{}); err == nil {
		t.Error("publish into the wrong shape must fail")
	}
	if _, err := publishOne[int]("not a number"); err == nil {
		t.Error("publishOne into the wrong shape must fail")
	}
	if got, err := publishOne[map[string]any](map[string]any{"a": 1}); err != nil || got["a"] != float64(1) {
		t.Errorf("publishOne = %v, %v", got, err)
	}
}

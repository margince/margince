// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// One 404 served two states: a day the nightly run has not reached, and an
// installation that has never built a digest. Asking for a past or future day
// said the product had never run.

import (
	"net/http"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestADayWithNoDigestIsNotAnInstallationWithNone(t *testing.T) {
	t.Parallel()
	day := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)

	forDay := missingDigest(&day)
	if forDay.Code != "no_digest_for_date" {
		t.Errorf("code = %q, want no_digest_for_date", forDay.Code)
	}
	if forDay.Status != http.StatusNotFound {
		t.Errorf("status = %d, want 404", forDay.Status)
	}
	// The day is named, because the caller asked for one and the answer is
	// about that day rather than about the installation.
	if !strings.Contains(forDay.Detail, "2026-10-09") {
		t.Errorf("detail = %q, want the day named", forDay.Detail)
	}

	atAll := missingDigest(nil)
	if atAll.Code != "no_digest_yet" {
		t.Errorf("code = %q, want no_digest_yet for a caller who named no day", atAll.Code)
	}
	if strings.Contains(atAll.Detail, "2026") {
		t.Errorf("detail = %q, must name no day when none was asked for", atAll.Detail)
	}
}

// An empty `date=` is a malformed parameter, not a request for the latest and
// not a day that happens to have no digest.
func TestAnEmptyDateParameterIsRefusedRatherThanDated(t *testing.T) {
	t.Parallel()
	zero := openapi_types.Date{}

	if err := refuseAnEmptyDate(crmcontracts.GetMorningDigestParams{Date: &zero}); err == nil {
		t.Fatal("an empty date was accepted, and it names no day")
	}

	// Absent and present stay as they were.
	if err := refuseAnEmptyDate(crmcontracts.GetMorningDigestParams{}); err != nil {
		t.Errorf("absent date refused with %v, and omitting it asks for the latest", err)
	}
	if day := digestDay(crmcontracts.GetMorningDigestParams{}); day != nil {
		t.Errorf("absent date = %v, want nil so the latest is served", day)
	}
	asked := openapi_types.Date{Time: time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)}
	if err := refuseAnEmptyDate(crmcontracts.GetMorningDigestParams{Date: &asked}); err != nil {
		t.Errorf("a real date refused with %v", err)
	}
	if day := digestDay(crmcontracts.GetMorningDigestParams{Date: &asked}); day == nil || !day.Equal(asked.Time) {
		t.Errorf("asked date = %v, want the day carried through", day)
	}
}

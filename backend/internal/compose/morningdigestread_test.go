// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// One 404 served two states: a day the nightly run has not reached, and an
// installation that has never built a digest. Asking for a past or future day
// said the product had never run.

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
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

	if err := refuseAnUnreadableDate(crmcontracts.GetMorningDigestParams{Date: &zero}); err == nil {
		t.Fatal("an empty date was accepted, and it names no day")
	}

	// Absent and present stay as they were.
	if err := refuseAnUnreadableDate(crmcontracts.GetMorningDigestParams{}); err != nil {
		t.Errorf("absent date refused with %v, and omitting it asks for the latest", err)
	}
	if day := digestDay(crmcontracts.GetMorningDigestParams{}); day != nil {
		t.Errorf("absent date = %v, want nil so the latest is served", day)
	}
	asked := openapi_types.Date{Time: time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)}
	if err := refuseAnUnreadableDate(crmcontracts.GetMorningDigestParams{Date: &asked}); err != nil {
		t.Errorf("a real date refused with %v", err)
	}
	if day := digestDay(crmcontracts.GetMorningDigestParams{Date: &asked}); day == nil || !day.Equal(asked.Time) {
		t.Errorf("asked date = %v, want the day carried through", day)
	}
}

// A year Go reads and Postgres cannot store is refused with the field named.
// It used to reach the store and answer as a server fault.
func TestADigestDayOutsideTheAcceptedRangeIsRefusedWithItsField(t *testing.T) {
	t.Parallel()
	for _, day := range []time.Time{
		time.Date(0, time.January, 1, 0, 0, 0, 0, time.UTC),
		time.Date(0, time.December, 31, 0, 0, 0, 0, time.UTC),
		time.Date(9999, time.December, 31, 0, 0, 0, 0, time.UTC),
	} {
		asked := openapi_types.Date{Time: day}
		err := refuseAnUnreadableDate(crmcontracts.GetMorningDigestParams{Date: &asked})
		var refusal *httperr.DetailedError
		if !errors.As(err, &refusal) || refusal.Status != http.StatusUnprocessableEntity ||
			len(refusal.Fields) != 1 || refusal.Fields[0].Field != "date" || refusal.Fields[0].Code != "out_of_range" {
			t.Errorf("%s → %v, want a 422 out_of_range on date", day.Format(time.DateOnly), err)
		}
	}
}

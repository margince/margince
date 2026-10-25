// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// Both precision refusals reach the wire as a 422 naming the field to change.
func TestPrecisionRefusalsAnswer422NamingTheField(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		err         error
		field, code string
	}{
		"a day on a held slot":               {&DayHeldSlotError{}, fieldOccurredPrecision, "day_holds_host_slot"},
		"a day with no date":                 {&RequiredFieldError{Field: auditFieldOccurredAt}, auditFieldOccurredAt, codeRequired},
		"an unknown precision":               {&InvalidPrecisionError{}, fieldOccurredPrecision, codeInvalidEnum},
		"a day made an instant with no time": {&DayToInstantNeedsTimeError{}, auditFieldOccurredAt, codeRequired},
	} {
		rec := httptest.NewRecorder()
		httperr.Write(rec, httptest.NewRequest(http.MethodPost, "/v1/activities", nil), c.err)
		body := rec.Body.String()
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s answered %d, want 422: %s", name, rec.Code, body)
		}
		if !strings.Contains(body, `"field":"`+c.field+`"`) || !strings.Contains(body, `"code":"`+c.code+`"`) {
			t.Errorf("%s does not name %s / %s: %s", name, c.field, c.code, body)
		}
	}
}

// The event names the precision only when it changed. A day moved to another
// date keeps its precision, and a new time on an instant implies one.
func TestTheUpdateEventNamesThePrecisionOnlyWhenItChanged(t *testing.T) {
	t.Parallel()
	day, instant := PrecisionDay, PrecisionInstant
	stored := func(precision string) crmcontracts.Activity {
		return crmcontracts.Activity{OccurredPrecision: crmcontracts.OccurredPrecision(precision)}
	}
	for name, c := range map[string]struct {
		in      UpdateActivityInput
		current crmcontracts.Activity
		want    string
	}{
		"a day moved to a day":      {UpdateActivityInput{OccurredPrecision: &day}, stored(PrecisionDay), ""},
		"an instant given a time":   {UpdateActivityInput{OccurredPrecision: &instant}, stored(PrecisionInstant), ""},
		"an instant made a day":     {UpdateActivityInput{OccurredPrecision: &day}, stored(PrecisionInstant), PrecisionDay},
		"a day given a real time":   {UpdateActivityInput{OccurredPrecision: &instant}, stored(PrecisionDay), PrecisionInstant},
		"a patch naming no time at": {UpdateActivityInput{}, stored(PrecisionDay), ""},
	} {
		got := ""
		if fields := activityUpdatedChangedFields(c.in, c.current); fields.OccurredPrecision != nil {
			got = string(*fields.OccurredPrecision)
		}
		if got != c.want {
			t.Errorf("%s: the event names %q, want %q", name, got, c.want)
		}
	}
}

// Wherever the audit images carry the time, they carry its precision on both
// sides. A day moved to another date then still reads as a date.
func TestTheAuditImagesCarryThePrecisionBesideTheTime(t *testing.T) {
	t.Parallel()
	before := crmcontracts.Activity{OccurredAt: time.Date(2026, 3, 9, 12, 0, 0, 0, time.UTC), OccurredPrecision: crmcontracts.OccurredPrecisionOccurredPrecisionDay}
	after := before
	after.OccurredAt = after.OccurredAt.AddDate(0, 0, 2)
	was, now := storekit.ChangedColumns(activityColumnImage(before), activityColumnImage(after))
	precisionBesideTime(was, now, before, after)
	if was[fieldOccurredPrecision] != PrecisionDay || now[fieldOccurredPrecision] != PrecisionDay {
		t.Errorf("a day moved to a day audits %v -> %v, want day on both sides", was[fieldOccurredPrecision], now[fieldOccurredPrecision])
	}

	renamed := before
	renamed.Subject = new("Fair, day two")
	was, now = storekit.ChangedColumns(activityColumnImage(before), activityColumnImage(renamed))
	precisionBesideTime(was, now, before, renamed)
	if _, named := now[fieldOccurredPrecision]; named {
		t.Errorf("a rename audits the precision: %v", now)
	}
}

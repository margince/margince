// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

// Both precision refusals reach the wire as a 422 naming the field to change.
func TestPrecisionRefusalsAnswer422NamingTheField(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		err         error
		field, code string
	}{
		"a day on a held slot": {&DayHeldSlotError{}, fieldOccurredPrecision, "day_holds_host_slot"},
		"a day with no date":   {&RequiredFieldError{Field: auditFieldOccurredAt}, auditFieldOccurredAt, codeRequired},
		"an unknown precision": {&InvalidPrecisionError{}, fieldOccurredPrecision, codeInvalidEnum},
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

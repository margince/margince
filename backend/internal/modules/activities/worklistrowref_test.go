// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
)

func TestMalformedWorklistRowsNameTheFieldBeforeWriting(t *testing.T) {
	for _, tc := range []struct {
		row         WorklistRowRef
		field, code string
	}{
		{WorklistRowRef{}, "source", "required"},
		{WorklistRowRef{Source: "made_up", RowID: "x"}, "source", "invalid_enum"},
		{WorklistRowRef{Source: "deal_at_risk"}, "row_id", "required"},
		{WorklistRowRef{Source: "awaiting_reply", RowID: "not-an-id"}, "row_id", "invalid_uuid"},
		{WorklistRowRef{Source: "customer_waiting", RowID: "not-an-id"}, "row_id", "invalid_uuid"},
		{WorklistRowRef{Source: "meeting_follow_up", RowID: "not-an-id"}, "row_id", "invalid_uuid"},
	} {
		fault, ok := httperr.Classify(tc.row.Validate("rows[0]."))
		if !ok || fault.Status != http.StatusUnprocessableEntity || len(fault.Fields) != 1 {
			t.Errorf("%+v answered %+v, want a named 422", tc.row, fault)
			continue
		}
		if field := fault.Fields[0]; field.Field != "rows[0]."+tc.field || field.Code != tc.code {
			t.Errorf("%+v named %+v, want %s/%s", tc.row, field, tc.field, tc.code)
		}
	}
}

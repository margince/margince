// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package apptest

import (
	"net/http"
	"testing"
)

func TestTheDefaultSourceFillsOnlyAnOmittedOneOnACreateRoute(t *testing.T) {
	t.Parallel()
	var nilMap map[string]any

	if got, ok := withRecordSource(http.MethodPost, "/v1/leads", nilMap).(map[string]any); !ok || got["source"] != "manual" {
		t.Errorf("a nil create body = %v, want it to gain source manual without panicking", got)
	}
	stated := map[string]any{"source": ""}
	if got, ok := withRecordSource(http.MethodPost, "/v1/leads", stated).(map[string]any); !ok || got["source"] != "" {
		t.Errorf("a stated empty source = %v, want it kept so a refusal test can send it", got)
	}
	omitted := map[string]any{"full_name": "Lee"}
	if got := withRecordSource(http.MethodPost, "/v1/leads", omitted).(map[string]any); got["source"] != "manual" || len(omitted) != 1 {
		t.Errorf("an omitted source: got %v, input %v; want the copy to gain it and the input to stay", got, omitted)
	}
	if got := withRecordSource(http.MethodPatch, "/v1/leads", omitted).(map[string]any); got["source"] != nil {
		t.Errorf("a PATCH gained a source: %v", got)
	}
	if got := withRecordSource(http.MethodPost, "/v1/contacts", omitted).(map[string]any); got["source"] != nil {
		t.Errorf("a route outside the list gained a source: %v", got)
	}
}

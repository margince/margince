// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A list under a record answers 404 for an id no record holds, even to a caller
// who sees every record. A record that exists answers its list.
func TestARecordsSubListAnswersNotFoundForARecordThatDoesNotExist(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Missing Parent", "missing-parent@fable.test", "Admin")
	stages := apptest.DiscoverSeededPipeline(t, e)
	deal := createdRecord(t, e, "/v1/deals", AnyMap{"name": "Parent deal", "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "source": "manual"})
	lead := createdRecord(t, e, "/v1/leads", AnyMap{"source": "manual", "full_name": "Stop Reader"})
	contact := createdRecord(t, e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Stop Holder", "linkedin_url": apptest.ContactProfile()})
	nowhere := ids.NewV7().String()
	for _, route := range []struct{ path, existing string }{
		{"/v1/deals/%s/products", deal},
		{"/v1/deals/%s/margince-changes", deal},
		{"/v1/deals/%s/lines", deal},
		{"/v1/leads/%s/consent/stops", lead},
		{"/v1/contacts/%s/consent/stops", contact},
	} {
		if status := e.Call(t, "GET", fmt.Sprintf(route.path, nowhere), nil, nil, nil); status != http.StatusNotFound {
			t.Errorf("GET %s for an id no record holds = %d, want 404", route.path, status)
		}
		if status := e.Call(t, "GET", fmt.Sprintf(route.path, route.existing), nil, nil, nil); status != http.StatusOK {
			t.Errorf("GET %s for a record that exists = %d, want 200", route.path, status)
		}
	}
}

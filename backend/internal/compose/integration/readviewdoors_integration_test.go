// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// read_view answers each view as its route answers it, because it runs that
// route's own handler as the caller. It is checked on a record's derived view,
// one reference record and a list narrowed by a filter.
func TestReadViewAnswersAsItsRoutes(t *testing.T) {
	d := newTwoDoors(t, "read-view-doors", "read")
	stages := apptest.DiscoverSeededPipeline(t, d.e)
	company := createdRecord(t, d.e, "/v1/companies", AnyMap{"source": "manual", "display_name": "View Works"})

	sameDoorRecords(t, "company_360",
		d.rest(t, "GET", "/v1/companies/"+company+"/360", nil),
		d.tool(t, "read_view", map[string]any{"view": "company_360", "id": company}), company)
	sameDoorRecords(t, "stages",
		d.rest(t, "GET", "/v1/stages?pipeline_id="+stages.PipelineID, nil),
		d.tool(t, "read_view", map[string]any{"view": "stages", "pipeline_id": stages.PipelineID}), stages.Open)
	sameDoorRecords(t, "pipeline",
		d.rest(t, "GET", "/v1/pipelines/"+stages.PipelineID, nil),
		d.tool(t, "read_view", map[string]any{"view": "pipeline", "id": stages.PipelineID}), stages.PipelineID)
}

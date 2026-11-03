// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"errors"

	"github.com/margince/margince/backend/internal/modules/agents/apps"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// readImport fronts the reads an import needs: one run, and the format its
// files take.
type readImport struct{ routed }

func (readImport) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "read_import", Title: "Read an import run or the import format", Version: toolVersionV1,
		Description: readImportCopy.render(),
		InputSchema: schema(`{"type":"object","properties":{
			"view":{"description":"report is one run's state and, once validated, what it will do or did; format is the files and columns an import takes"}}}`),
	}
}

func (readImport) routeField() string { return "view" }

func (readImport) routeTargets() map[string]string {
	return map[string]string{"report": "read_import_report", "format": "describe_import_format"}
}

// readForecast fronts the forecast's figures, the account of how they moved, and
// whether its inputs can be trusted.
type readForecast struct{ routed }

func (readForecast) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "read_forecast", Title: "Read the forecast and how it moved", Version: toolVersionV1,
		Description: readForecastCopy.render(),
		Instead:     readForecastCopy.Instead,
		InputSchema: schema(`{"type":"object","properties":{
			"view":{"description":"readings: what a period is expected to close; movement: why it changed between two snapshots; input_summary, input_exceptions and input_sources: whether its inputs can be trusted"}}}`),
		UI: &mcp.ToolUI{ResourceURI: apps.ForecastURI, Visibility: []string{mcp.VisibilityModel}},
	}
}

func (readForecast) routeField() string { return "view" }

func (readForecast) routeTargets() map[string]string {
	return map[string]string{
		"readings": "forecast_readings", "movement": "forecast_movement",
		"input_summary": "forecast_input_checks", "input_exceptions": "list_input_checks", "input_sources": "data_coverage",
	}
}

type readRelationships struct{ routed }

func (readRelationships) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "read_relationships", Title: "Read who we know, and how well", Version: toolVersionV1,
		Description: readRelationshipsCopy.render(),
		Instead:     readRelationshipsCopy.Instead,
		InputSchema: schema(`{"type":"object","properties":{
			"contact_id":{"description":"The colleagues who know this contact; name exactly one of the three ids"},
			"company_id":{"description":"A warm route into this company; name exactly one of the three ids"},
			"deal_id":{"description":"Which roles at this deal's company we cover; name exactly one of the three ids"}}}`),
	}
}

// relationshipCompanyID is the argument a warm-route read takes.
const relationshipCompanyID = "company_id"

// relationshipTargetByID is each target by the one id it takes.
var relationshipTargetByID = map[string]string{
	"contact_id":          "who_knows",
	relationshipCompanyID: "intro_path_to",
	"deal_id":             "company_coverage",
}

func (readRelationships) routeTargets() map[string]string { return relationshipTargetByID }

var errRelationshipsNamesOneID = &BadArgsError{Cause: errors.New("name exactly one of contact_id, company_id or deal_id")}

func (readRelationships) chooseTarget(members map[string]json.RawMessage) (string, error) {
	var target string
	for id, tool := range relationshipTargetByID {
		dropNullMembers(members, id)
		if _, named := members[id]; !named {
			continue
		}
		if target != "" {
			return "", errRelationshipsNamesOneID
		}
		target = tool
	}
	if target == "" {
		return "", errRelationshipsNamesOneID
	}
	return target, nil
}

// readProject fronts the two reads of one project: its whole page, and what its
// delivery side needs from sales.
type readProject struct{ routed }

func (readProject) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "read_project", Title: "Read a project or its handover", Version: toolVersionV1,
		Description: readProjectCopy.render(),
		Instead:     readProjectCopy.Instead,
		InputSchema: schema(`{"type":"object","properties":{
			"view":{"description":"overview is the project's whole page; handoff is what delivery needs from sales, with its gaps"}}}`),
	}
}

func (readProject) routeField() string { return "view" }

func (readProject) routeTargets() map[string]string {
	return map[string]string{"overview": "read_project_360", "handoff": "prepare_handoff"}
}

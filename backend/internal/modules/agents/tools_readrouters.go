// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"errors"

	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// readInputChecks fronts the three answers to whether the forecast's inputs can
// be trusted. Each stays its own tool because each is its own REST route.
type readInputChecks struct{ routed }

func (readInputChecks) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "read_input_checks", Title: "Can the forecast's inputs be trusted", Version: toolVersionV1,
		Description: readInputChecksCopy.render(),
		InputSchema: schema(`{"type":"object","properties":{
			"view":{"description":"summary is the verdict, exceptions the open problems, sources what could be read"}}}`),
	}
}

func (readInputChecks) routeField() string { return "view" }

func (readInputChecks) routeTargets() map[string]string {
	return map[string]string{
		"summary":    "forecast_input_checks",
		"exceptions": "list_input_checks",
		"sources":    "data_coverage",
	}
}

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

// readForecast fronts the forecast's figures and the account of how they moved.
type readForecast struct{ routed }

func (readForecast) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "read_forecast", Title: "Read the forecast and how it moved", Version: toolVersionV1,
		Description: readForecastCopy.render(),
		Instead:     readForecastCopy.Instead,
		InputSchema: schema(`{"type":"object","properties":{
			"view":{"description":"readings is what a period is expected to close; movement is why it changed between two snapshots"}}}`),
	}
}

func (readForecast) routeField() string { return "view" }

func (readForecast) routeTargets() map[string]string {
	return map[string]string{"readings": "forecast_readings", "movement": "forecast_movement"}
}

// readDirectory fronts the workspace's own set-up: who holds a seat, its
// pipelines and stages, and the messaging transports it composed. whoami stays
// apart: it is offered to every passport, and a router spends one scope.
type readDirectory struct{ routed }

func (readDirectory) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "read_directory", Title: "Read the workspace's colleagues, pipelines and channels", Version: toolVersionV1,
		Description: readDirectoryCopy.render(),
		Instead:     readDirectoryCopy.Instead,
		InputSchema: schema(`{"type":"object","properties":{
			"list":{"description":"colleagues are the seats here, pipelines the stages a deal moves through, channel_providers the messaging transports"}}}`),
	}
}

func (readDirectory) routeField() string { return "list" }

func (readDirectory) routeTargets() map[string]string {
	return map[string]string{
		"colleagues":        "list_colleagues",
		"pipelines":         "list_pipelines",
		"channel_providers": "list_channel_providers",
	}
}

// readRelationships fronts the relationship reads that start from one record.
// The id a call names says which: each target takes a different one.
// at_risk_relationships stays apart: a scheduled sweep is allowed it alone.
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

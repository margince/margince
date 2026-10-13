// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import "github.com/margince/margince/backend/internal/shared/ports/mcp"

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
// pipelines and stages, and the messaging transports it composed.
type readDirectory struct{ routed }

func (readDirectory) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "read_directory", Title: "Read the workspace's colleagues, pipelines and channels", Version: toolVersionV1,
		Description: readDirectoryCopy.render(),
		Instead:     readDirectoryCopy.Instead,
		InputSchema: schema(`{"type":"object","properties":{
			"of":{"description":"colleagues are the seats here, pipelines the stages a deal moves through, channel_providers the messaging transports"}}}`),
	}
}

func (readDirectory) routeField() string { return "of" }

func (readDirectory) routeTargets() map[string]string {
	return map[string]string{
		"colleagues":        "list_colleagues",
		"pipelines":         "list_pipelines",
		"channel_providers": "list_channel_providers",
	}
}

// readRelationships fronts the relationship graph read from one record.
// at_risk_relationships stays apart: a scheduled sweep is allowed it alone.
type readRelationships struct{ routed }

func (readRelationships) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "read_relationships", Title: "Read who we know, and how well", Version: toolVersionV1,
		Description: readRelationshipsCopy.render(),
		Instead:     readRelationshipsCopy.Instead,
		InputSchema: schema(`{"type":"object","properties":{
			"view":{"description":"knows takes contact_id, intro company_id, coverage deal_id"}}}`),
	}
}

func (readRelationships) routeField() string { return "view" }

func (readRelationships) routeTargets() map[string]string {
	return map[string]string{"knows": "who_knows", "intro": "intro_path_to", "coverage": "company_coverage"}
}

// readContext fronts the assembled pictures a person reads before acting: a
// meeting brief, a project handover and a project's whole page.
type readContext struct{ routed }

func (readContext) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "read_context", Title: "Prepare for a meeting, handover or project", Version: toolVersionV1,
		Description: readContextCopy.render(),
		Instead:     readContextCopy.Instead,
		InputSchema: schema(`{"type":"object","properties":{
			"for":{"description":"meeting prepares for one record or meeting; handoff and project read one project by project_id"}}}`),
	}
}

func (readContext) routeField() string { return "for" }

func (readContext) routeTargets() map[string]string {
	return map[string]string{"meeting": "prep_for_meeting", "handoff": "prepare_handoff", "project": "read_project_360"}
}

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

// readImport fronts the two reads of one import run.
type readImport struct{ routed }

func (readImport) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "read_import", Title: "Read an import run", Version: toolVersionV1,
		Description: readImportCopy.render(),
		InputSchema: schema(`{"type":"object","properties":{
			"view":{"description":"run is where it got to, report what it will do or did"}}}`),
	}
}

func (readImport) routeField() string { return "view" }

func (readImport) routeTargets() map[string]string {
	return map[string]string{"run": "read_import_run", "report": "read_import_report"}
}

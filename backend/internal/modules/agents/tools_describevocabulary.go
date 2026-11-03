// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import "github.com/margince/margince/backend/internal/shared/ports/mcp"

// readWorkspace fronts how this workspace is set up: the names each tool
// accepts, who holds a seat, its pipelines and stages, and its messaging
// transports. whoami stays apart, offered to every passport.
type readWorkspace struct{ routed }

func (readWorkspace) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "read_workspace", Title: "Read how this workspace is set up", Version: toolVersionV1,
		Description: readWorkspaceCopy.render(),
		Instead:     readWorkspaceCopy.Instead,
		InputSchema: schema(`{"type":"object","properties":{
			"topic":{"description":"colleagues, pipelines or channel_providers; or which names a tool accepts: query, analytics, record_fields, list_filters, report_blocks, report_vocabulary"}}}`),
	}
}

// topicQuery is the topic naming query_workspace's vocabulary.
const topicQuery = "query"

func (readWorkspace) routeField() string { return "topic" }

func (readWorkspace) routeTargets() map[string]string {
	return map[string]string{
		topicQuery:          "describe_query_vocabulary",
		"analytics":         "describe_analytics_vocabulary",
		"record_fields":     "describe_record_fields",
		"list_filters":      "describe_list_filters",
		"report_blocks":     "describe_report_blocks",
		"report_vocabulary": "describe_report_vocabulary",
		"colleagues":        "list_colleagues",
		"member_retirement": "preview_member_retirement",
		"pipelines":         "list_pipelines",
		"channel_providers": "list_channel_providers",
	}
}

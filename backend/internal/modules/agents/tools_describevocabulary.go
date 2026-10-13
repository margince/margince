// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import "github.com/margince/margince/backend/internal/shared/ports/mcp"

// describeVocabulary fronts the five documents that name what another tool
// accepts. Each stays its own tool because each is its own REST route.
type describeVocabulary struct{ routed }

func (describeVocabulary) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "describe_vocabulary", Title: "Describe what a tool accepts", Version: toolVersionV1,
		Description: describeVocabularyCopy.render(),
		Instead:     describeVocabularyCopy.Instead,
		InputSchema: schema(`{"type":"object","properties":{
			"topic":{"description":"Which tool's names to describe"}}}`),
	}
}

// topicQuery is the topic naming query_workspace's vocabulary.
const topicQuery = "query"

func (describeVocabulary) routeField() string { return "topic" }

func (describeVocabulary) routeTargets() map[string]string {
	return map[string]string{
		topicQuery:          "describe_query_vocabulary",
		"analytics":         "describe_analytics_vocabulary",
		"record_fields":     "describe_record_fields",
		"report_blocks":     "describe_report_blocks",
		"report_vocabulary": "describe_report_vocabulary",
	}
}

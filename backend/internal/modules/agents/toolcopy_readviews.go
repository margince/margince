// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Written copy for the read_view catalog. See toolcopy.go for what each field
// answers.

var describeViewsCopy = toolCopy{
	Purpose: "Answer which views read_view reads: each view's name, the path arguments it needs " +
		"and the query arguments it takes, with their JSON types. It is the catalog read_view " +
		"refuses against.",
	Limits: "It describes the views and reads none; read_view does that. It is not a " +
		"prerequisite: a caller that knows a view's name calls read_view directly.",
	Instead: "Call read_view directly when the view is known. An unknown name is refused with " +
		"every view there is. This tool answers the same document as the margince://schema/views " +
		"resource, for a caller that reads tools rather than resources.",
	Retain: "Take a view's name and its argument names verbatim.",
}

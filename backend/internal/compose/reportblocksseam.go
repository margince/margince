// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The block grammar, handed to the tool surface from the package that owns it.
//
// reportdoc decides what a document may contain and refuses against exactly
// this list. Passing it through rather than restating it in agents/ is what
// keeps one grammar: a kind added to reportdoc appears in the published
// document without anybody remembering to add it twice.

import (
	"github.com/margince/margince/backend/internal/compose/reportdoc"
	"github.com/margince/margince/backend/internal/modules/agents"
)

// reportBlockGrammar renders reportdoc's own catalog in the shape the tool
// surface publishes.
//
// The two structs are separate because agents/ may not import compose — the
// module DAG forbids it — so this is the seam, and its only job is the
// field-for-field carry. Nothing here decides anything.
func reportBlockGrammar() agents.BlockGrammar {
	described := reportdoc.Catalog()
	blocks := make([]agents.BlockDescription, 0, len(described))
	for _, b := range described {
		blocks = append(blocks, agents.BlockDescription{
			Kind:          b.Kind,
			TakesCells:    b.TakesCells,
			TakesText:     b.TakesText,
			TakesSeverity: b.TakesSeverity,
		})
	}
	return agents.BlockGrammar{Blocks: blocks, Severities: reportdoc.Severities()}
}

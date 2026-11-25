// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// importNameTargets are the cells that name a record, for every type a file
// may hold. A line break or a run of spaces in one is a slip in the file.
var importNameTargets = []string{fieldFullName, "first_name", "last_name", fieldDisplayName, fieldLegalName, dealTargetName}

// foldNameCells folds each run of white space in a name cell to one space.
func foldNameCells(fields map[string]any) {
	for _, target := range importNameTargets {
		if name, ok := fields[target].(string); ok {
			fields[target] = values.CollapseSpace(name)
		}
	}
}

// shapeCells sets the rewrites every reader of a part's rows sees, so the
// test run and the commit check and write one spelling. Names fold first, then
// the address cells change as the mapping asks.
func shapeCells(source *migration.CSVSource, object string, mapping map[string]string) {
	address := addressShape(object, mapping)
	source.ShapeRows(func(fields map[string]any) {
		foldNameCells(fields)
		if address != nil {
			address(fields)
		}
	})
}

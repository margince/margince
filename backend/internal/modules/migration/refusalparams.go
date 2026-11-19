// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import "slices"

// RefusalParam names a value an import refusal carries in its params.
type RefusalParam string

// The values import refusals name: a file refusal's here, a mapping's in
// compose's importwire.
const (
	ParamReason      RefusalParam = "reason"
	ParamFile        RefusalParam = "file"
	ParamOtherFile   RefusalParam = "other_file"
	ParamLine        RefusalParam = "line"
	ParamPosition    RefusalParam = "position"
	ParamName        RefusalParam = "name"
	ParamObject      RefusalParam = "object"
	ParamFiles       RefusalParam = "files"
	ParamMaxFiles    RefusalParam = "max_files"
	ParamMaxMB       RefusalParam = "max_mb"
	ParamRows        RefusalParam = "rows"
	ParamTarget      RefusalParam = "target"
	ParamColumn      RefusalParam = "column"
	ParamOtherColumn RefusalParam = "other_column"
	ParamRefTarget   RefusalParam = "ref_target"
)

var refusalParams = []RefusalParam{
	ParamReason, ParamFile, ParamOtherFile, ParamLine, ParamPosition, ParamName, ParamObject, ParamFiles,
	ParamMaxFiles, ParamMaxMB, ParamRows, ParamTarget, ParamColumn, ParamOtherColumn, ParamRefTarget,
}

// RefusalValues are a refusal's values keyed by name, as its params carry them.
func RefusalValues(values map[RefusalParam]string) Params {
	params := make(Params, len(values)+1)
	for name, value := range values {
		params[string(name)] = value
	}
	return params
}

// RefusalParams answers every value name an import refusal may carry, sorted.
func RefusalParams() []RefusalParam {
	return slices.Sorted(slices.Values(refusalParams))
}

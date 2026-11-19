// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importwire

// The import refusal vocabulary, for the gates that hold the browser's copy of
// it. Its owner is migration, which a gate may not import.

import "github.com/margince/margince/backend/internal/modules/migration"

// RunStateCode refuses a save on a run that is no longer a draft.
const RunStateCode = migration.RunStateCode

// FileReasons are the reasons a file refusal names, sorted.
func FileReasons() []string {
	reasons := migration.FileFaultReasons()
	out := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		out = append(out, string(reason))
	}
	return out
}

// RefusalParams are the names of the values an import refusal carries, sorted.
func RefusalParams() []string {
	params := migration.RefusalParams()
	out := make([]string, 0, len(params))
	for _, param := range params {
		out = append(out, string(param))
	}
	return out
}

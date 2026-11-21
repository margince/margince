// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importwire

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// StopOf is the stop of a test run refused for fault. It keeps the sentence and
// the code and values of the field error, which a screen words in the reader's
// language. A fault that names no field keeps the sentence alone.
func StopOf(fault httperr.Fault, err error) migration.RunStop {
	stop := migration.RunStop{Cause: fault.Detail, Err: err}
	if len(fault.Fields) > 0 {
		field := fault.Fields[0]
		stop.Refusal = &migration.StopRefusal{Field: field.Field, Code: field.Code, Max: field.Max, Params: field.Params}
	}
	return stop
}

// RunRefusal is a run's stored refusal on the wire, nil when it has none.
func RunRefusal(refusal *migration.StopRefusal) *crmcontracts.ImportRunRefusal {
	if refusal == nil {
		return nil
	}
	out := &crmcontracts.ImportRunRefusal{Field: refusal.Field, Code: refusal.Code}
	if refusal.Max > 0 {
		out.Max = &refusal.Max
	}
	if len(refusal.Params) > 0 {
		out.Params = &refusal.Params
	}
	return out
}

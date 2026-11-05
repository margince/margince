// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// readServable reads back, through the seam and without stamping it, a record
// another engine selected. Every tool that serves such records decides here
// whether one is still the caller's to see.
//
// The bool is the verdict: false drops the record. An error is no verdict, and
// is returned, so an unreachable store never reads as a partial page.
//
// A record whose content the caller may no longer read is unreadable too. It
// was selected on that content, and serving it would hand back what is withheld.
func readServable(ctx context.Context, p datasource.SystemOfRecordProvider, ref datasource.EntityRef) (datasource.Record, bool, error) {
	record, err := p.Read(ctx, ref)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
			return datasource.Record{}, false, nil
		}
		return datasource.Record{}, false, err
	}
	withheld, err := contentWithheld(record)
	if err != nil || withheld {
		return datasource.Record{}, false, err
	}
	return record, true, nil
}

// contentWithheld reads the content_state an activity carries when the caller
// may know it exists but not read it. Other records carry none.
func contentWithheld(record datasource.Record) (bool, error) {
	if len(record.Fields) == 0 {
		return false, nil
	}
	var state struct {
		ContentState *crmcontracts.ActivityContentState `json:"content_state"`
	}
	if err := json.Unmarshal(record.Fields, &state); err != nil {
		return false, fmt.Errorf("agents: reading a record's content state: %w", err)
	}
	return state.ContentState != nil && *state.ContentState == crmcontracts.ActivityContentStateWithheld, nil
}

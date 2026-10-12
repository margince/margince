// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package accountdraft

import (
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// An account known only from a logged note is not written to as a stranger,
// and silence_days counts from the note: the shared TIME rule says so to every
// surface, so this one has to compute it that way too.
func TestANoteOnlyAccountIsNotAFirstTouch(t *testing.T) {
	now := time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)
	body := "Met their COO at the trade fair."
	view := crmcontracts.Company360{Activities: &crmcontracts.ActivityListResponse{Data: []crmcontracts.Activity{{
		Id: openapi_types.UUID(ids.NewV7()), Kind: crmcontracts.ActivityKindNote,
		Body: &body, OccurredAt: now.Add(-10 * 24 * time.Hour),
	}}}}

	state := ConversationState(view, now)
	if state.Band == convstate.BandNone || state.SilenceDays != 10 {
		t.Errorf("state = %+v, want ten days since the note", state)
	}
}

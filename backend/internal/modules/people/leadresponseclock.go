// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// When a lead's response clocks start and stop.
//
// Two stamps, both about the same promise: routed_at says when somebody became
// answerable for the lead, and first_response_at says when they answered. The
// SLA reads COALESCE(routed_at, created_at), so which of them is set — and
// when — is what decides whether a lead reads as breached.
//
// Split out of lead_update.go because it is one concept with its own rules, and
// because that file had grown past the length a reader can hold at once.

package people

import (
	"context"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// stampHumanFirstResponse adds the §18.1 first-response stamp to a patch
// that moves the lead off `new` by a HUMAN's hand. An agent's status change
// is not a response, and a lead already answered keeps its first stamp.
func stampHumanFirstResponse(ctx context.Context, p *storekit.Patch, current crmcontracts.Lead, in UpdateLeadInput) error {
	if in.Status == nil || current.Status != crmcontracts.LeadStatusNew || current.FirstResponseAt != nil {
		return nil
	}
	if LeadStatus(*in.Status) == LeadStatusNew {
		return nil
	}
	actor, err := storekit.Actor(ctx)
	if err != nil {
		return err
	}
	if actor.Type == principal.PrincipalHuman {
		p.Set(firstResponseColumn, nil, time.Now().UTC())
	}
	return nil
}

// leadStatusSetByColumn records who placed the lead on its current step.
const leadStatusSetByColumn = "status_set_by"

// stampStatusSetBy records that a status written through this path was a
// hand's doing — a human, or an agent acting for one — as opposed to the
// system's own climb from captured activity (advanceLeadStatusTx).

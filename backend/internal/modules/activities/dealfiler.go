// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Who filed an activity on a deal, for a link no human or agent chose. The
// answer is read from the latest relink row about the activity and the deal,
// the row capture's filing and a colleague's decline both write.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// dealFilerJoin adds the latest filing row of a deal link as filer, with its
// actor type and the first extension hook its evidence names. Other links
// join no row. alias is the activity_link alias of the statement.
func dealFilerJoin(alias string) string {
	return ` LEFT JOIN LATERAL (` + provenance.LatestDealFilingSQL(alias+".activity_id", alias+".deal_id") +
		`) filer(actor_type, hook) ON ` + alias + `.entity_type = 'deal'`
}

// dealFilerColumns are the filer's actor type, and the unit, hook and reason
// of its first extension hook.
const dealFilerColumns = `filer.actor_type, filer.hook->>'unit', filer.hook->>'hook', filer.hook->>'reason'`

// dealFiler answers who filed the link from its latest filing row. It is nil
// when there is none, or a human or an agent wrote it. Otherwise it is the
// extension whose hook shaped the row, or core.
func dealFiler(actorType, unit, hook, reason *string) *crmcontracts.LinkFiler {
	if actorType == nil || *actorType == "human" || *actorType == "agent" {
		return nil
	}
	if unit == nil || *unit == "" {
		return &crmcontracts.LinkFiler{Actor: "core"}
	}
	return &crmcontracts.LinkFiler{Actor: *unit, Hook: (*crmcontracts.LinkFilerHook)(hook), Reason: reason}
}

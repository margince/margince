// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
)

// contactCaseMerger binds merge_records' case arm to the contacts store the
// review screen posts to, so both doors run one decision.
type contactCaseMerger struct{ store *contacts.Store }

//nolint:ireturn // the seam is an interface by design: agents may not import the contacts module
func contactCaseMergerSeam(db *database.DB) agents.ContactCaseMerger {
	return contactCaseMerger{store: contacts.NewStore(db)}
}

func (m contactCaseMerger) PreviewContactCaseMerge(ctx context.Context, cmd agents.ContactCaseCommand) (crmcontracts.ContactCaseOutcome, error) {
	return m.store.PreviewContactCaseMerge(ctx, cmd.ContactID, caseDecisionOf(cmd))
}

func (m contactCaseMerger) MergeContactCase(ctx context.Context, cmd agents.ContactCaseCommand) (crmcontracts.ContactCaseOutcome, error) {
	return m.store.MergeContactCase(ctx, cmd.ContactID, caseDecisionOf(cmd))
}

func caseDecisionOf(cmd agents.ContactCaseCommand) contacts.ContactCaseDecision {
	return contacts.ContactCaseDecision{
		Revision: cmd.Revision, MemberIDs: cmd.MemberIDs, SurvivorID: cmd.SurvivorID, OwnerID: cmd.OwnerID,
	}
}

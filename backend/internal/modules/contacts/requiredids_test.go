// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A required body id the caller omitted must be named as a missing argument, not
// discovered as a missing row: an absent key decodes to the zero UUID with no
// error, so unguarded it reaches a lookup, matches nothing, and answers a bare
// not-found for a record the caller never mentioned.
//
// The guard is at the store entry point — the door every transport comes through —
// and it runs BEFORE any authority check or query, which is why these probes need
// no database and no actor: a store over a nil pool never reaches one.
//
// The refusal's SHAPE is proven once in platform/httperr/requirebodyid_test.go and
// asserted here through faulttest. What is left is the only question this package
// can answer: is the guard actually called for my body.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr/faulttest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAnOmittedMergeTargetOrStakeholderIsNamed(t *testing.T) {
	// MergeContactJSONBody.target_id, MergeCompanyJSONBody.target_id,
	// SetProjectStakeholderRequest.contact_id and
	// SetProjectCompanyRequest.company_id.
	//
	// The merge pair is the sharp case: the self-merge guard next to it does NOT
	// catch an omitted target, because a real source id never equals the zero
	// one — so the zero id reached the pair lock and answered not-found for a
	// survivor nobody named.
	store := NewStore(nil)
	ctx := context.Background()

	_, err := store.MergeContact(ctx, ids.New[ids.ContactKind](), ids.ContactID{})
	faulttest.AssertNamesOmittedID(t, err, "target_id")

	_, err = store.MergeCompany(ctx, ids.New[ids.CompanyKind](), ids.CompanyID{})
	faulttest.AssertNamesOmittedID(t, err, "target_id")

	_, err = store.SetProjectStakeholder(ctx, SetProjectStakeholderInput{
		ProjectID: ids.New[ids.ProjectKind](), Role: "sponsor",
	})
	faulttest.AssertNamesOmittedID(t, err, "contact_id")

	// SetProjectCompanyRequest.company_id, for the same reason: without
	// the guard the zero id reaches the company visibility probe, which answers
	// not-found — telling the caller a company they never named does not exist.
	_, err = store.SetProjectCompany(ctx, SetProjectCompanyInput{
		ProjectID: ids.New[ids.ProjectKind](), Role: "partner",
	})
	faulttest.AssertNamesOmittedID(t, err, "company_id")
}

func TestAnOmittedClaimSourceActivityIsNamed(t *testing.T) {
	// RecordConversationClaimRequest.source_activity_id.
	//
	// A claim that cites nothing is the case this guard exists for: the store
	// would otherwise carry the zero id into the activity visibility probe,
	// which answers not-found — telling the caller a message they never named
	// does not exist, when what actually happened is they forgot to name one.
	store := NewStore(nil)

	_, err := store.RecordConversationClaim(context.Background(), ClaimInput{
		ContactID: ids.New[ids.ContactKind](),
		Kind:      "commitment_ours",
		Body:      "send the revised model",
		Quote:     "I'll get you the model by Friday",
	})
	faulttest.AssertNamesOmittedID(t, err, "source_activity_id")
}

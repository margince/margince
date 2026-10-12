// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

import (
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// A request refused because a live run was already buying the same category
// is newer than that run. The page still tells the reader about the run doing
// the buying. Read as the newest run, the refusal would show "not eligible",
// and the page would stop watching the purchase still in flight.
func TestTheProfileFollowsTheRunARefusalDeferredTo(t *testing.T) {
	t.Parallel()

	live := providerRunRow{id: ids.NewV7(), state: string(provider.RunInProgress)}
	refused := providerRunRow{
		id: ids.NewV7(), state: string(provider.RunSkipped),
		skipReason: string(provider.SkipCategoryInFlight),
	}
	profile, err := (&Service{}).profileFor("surfe", "connected",
		[]providerRunRow{refused, live}, nil, provider.ContactIdentifiers{})
	if err != nil {
		t.Fatal(err)
	}
	if profile.State != crmcontracts.ContactProviderProfileStateInProgress {
		t.Errorf("state = %q, want in_progress from the run still buying", profile.State)
	}
	if profile.LatestRun == nil || profile.LatestRun.Id != openapi_types.UUID(live.id) {
		t.Errorf("latest run = %v, want the live run %s that the page must keep watching", profile.LatestRun, live.id)
	}
}

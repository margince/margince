// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

// Turning one timeline row into an Activity.
//
// Split out of readActivities because it IS a separate concept: that function
// decides which rows this contact may see and in what order, and this one
// decides what a row means once it arrives. Keeping them together pushed the
// file past the length cap, and the cap was right — the two change for
// different reasons and are read by readers asking different questions.
//
// This is a hand-written twin of activities.activityProjection, which is how
// the contact timeline came to be missing a column for a whole slice, twice.
// Every column the list learns to carry has to be taught here too, and
// TestTheContact360TimelineNamesTheTransportThatCarriedAMessage in
// compose/integration is what says so out loud — with two siblings beside it
// for the version and the transcript source, each a later instance of the same
// defect.

import (
	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// scanTimelineRow reads one row of the timeline query into an Activity.
//
// The shared projection does the reading and the folding; this adds the one
// column that is genuinely this page's own. A hand-written scan here was a
// second copy of a list that grows, and it silently failed to grow three
// times — so the only thing left to get wrong is the extra, which is one
// value and sits last by construction.
func scanTimelineRow(rows pgx.Rows, contactID ids.ContactID) (crmcontracts.Activity, error) {
	// Links say how the message is FILED, and a row can reach this page without
	// being filed here: a contact who was CC'd or who attended is on the message
	// through their participant row, while the filing belongs to whoever capture
	// named as its counterparty. Asserting a link for those would describe a row
	// activity_link does not hold — and a client acting on it, to unfile the
	// message, would act on nothing.
	var filedHere bool
	a, err := activities.ScanActivityRow(rows, &filedHere)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	if filedHere {
		a.Links = &[]crmcontracts.ActivityLink{{
			EntityType: crmcontracts.ActivityLinkEntityTypeContact,
			EntityId:   openapi_types.UUID(contactID.UUID),
		}}
	}
	return a, nil
}

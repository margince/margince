// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package providerwire

import (
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// Run renders one provider run the way the contract spells it. The run
// endpoint and the contact page both call it, so a skipped run reads the same
// on each.
func Run(r provider.Run) crmcontracts.ProviderRun {
	cats := make([]string, 0, len(r.RequestedCategories))
	for _, c := range r.RequestedCategories {
		cats = append(cats, string(c))
	}
	out := crmcontracts.ProviderRun{
		Id:                    uuidOf(r.ID),
		SubjectKind:           crmcontracts.ProviderRunSubjectKind(r.SubjectKind),
		Provider:              crmcontracts.Provider(r.Provider),
		Trigger:               crmcontracts.ProviderRunTrigger(r.Trigger),
		State:                 crmcontracts.ProviderRunState(r.State),
		ClaimsUnwritten:       r.ClaimsUnwritten,
		Applied:               &r.Applied,
		ConfigurationSnapshot: Snapshot(r.Snapshot),
		ConnectionVersion:     r.ConnectionVersion,
		RequestedCategories:   cats,
		Reservations:          []crmcontracts.ProviderRunReservation{},
		CreatedAt:             r.CreatedAt,
		UpdatedAt:             r.UpdatedAt,
		SubmittedAt:           r.SubmittedAt,
		CompletedAt:           r.CompletedAt,
	}
	if r.ContactID != "" {
		id := uuidOf(r.ContactID)
		out.ContactId = &id
	}
	if r.SkipReason != "" {
		reason := crmcontracts.ProviderRunSkipReason(r.SkipReason)
		out.SkipReason = &reason
	}
	if r.SafeStatusCode != "" {
		code := r.SafeStatusCode
		out.SafeStatusCode = &code
	}
	for _, res := range r.Reservations {
		out.Reservations = append(out.Reservations, crmcontracts.ProviderRunReservation{
			Pool: string(res.Pool), ReservedCredits: res.Reserved, ActualCredits: res.Actual,
		})
	}
	return out
}

// uuidOf parses an id the database produced. A malformed one is a programming
// error, not a caller's mistake. The zero uuid stands for an id that did not parse.
func uuidOf(s string) openapi_types.UUID {
	parsed, err := ids.Parse(s)
	if err != nil {
		return openapi_types.UUID{}
	}
	return openapi_types.UUID(parsed)
}

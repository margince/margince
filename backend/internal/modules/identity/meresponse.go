// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// What /me says about the caller, and the three mappings that render it.
//
// Split out of handlers.go, which had grown to hold both the HTTP verbs and the
// projection they answer with. The verbs are about a request — cookies,
// statuses, which capability a caller may ask for; this is about one struct's
// shape on the wire, and a reader asking either question was reading past the
// other.

import (
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// meResponse renders /me for one principal. It is a method rather than a
// function because every posture it reports beyond the identity itself — the
// deployment posture, whether this caller may issue set-password links — is
// wiring the composition root injected onto Handlers, so passing them
// alongside would be a row of anonymous booleans at each call site.
func (h Handlers) meResponse(
	id Identity,
	sorMode crmcontracts.MeResponseSystemOfRecordMode,
) crmcontracts.MeResponse {
	adminPasswordLink := h.canIssuePasswordLink(id)
	roles := id.Roles
	if roles == nil {
		roles = []string{}
	}
	teams := make([]openapi_types.UUID, len(id.Teams))
	for i, t := range id.Teams {
		teams[i] = openapi_types.UUID(t.UUID)
	}
	return crmcontracts.MeResponse{
		User: crmcontracts.User{
			Id:          openapi_types.UUID(id.UserID.UUID),
			Email:       openapi_types.Email(id.Email),
			DisplayName: id.DisplayName,
			Status:      "active",
			Locale:      contractLocale(id.Locale),
			// The round trip was built at both ends and severed here. Signup
			// captures the browser's zone, ParseTimezone validates it, and the
			// row stores it — and this response then dropped it, so nothing
			// downstream could localize an instant to the person reading it.
			// The twelve hard-coded "Europe/Berlin" literals on the frontend
			// are what a caller does when no correct answer is served.
			// margince/margince#26.
			Timezone: optionalString(id.Timezone),
		},
		Roles:         roles,
		Teams:         teams,
		WorkspaceName: id.WorkspaceName,
		SystemOfRecord: &struct {
			Mode crmcontracts.MeResponseSystemOfRecordMode `json:"mode"`
		}{Mode: sorMode},
		NonProduction:      h.nonProduction,
		DataResetAvailable: &h.dataResetAvailable,
		AdminPasswordLink:  adminPasswordLink,
		Authorization: &crmcontracts.Authorization{
			SeatType: contractSeatType(id.SeatType),
			Objects:  contractObjectGrants(id.Permissions.Objects),
		},
	}
}

// optionalString omits an empty value rather than sending it.
//
// The zone is `omitempty` in the contract, and absent has to stay
// distinguishable from chosen: UTC is a zone somebody can pick, so a seat that
// never stated one must not arrive as "UTC" — the client falls back to the
// browser only when the field is absent, and a default sent as a value takes
// that decision away from it.
func optionalString(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

// contractSeatType maps the stored seat onto the contract enum through the
// kernel's own predicate rather than a cast. Two properties follow that a cast
// would not give: the response can only ever carry a value the enum declares,
// and a seat the kernel does not recognize reports the ceiling that denies
// instead of the one that admits.
func contractSeatType(seat string) crmcontracts.AuthorizationSeatType {
	if principal.SeatType(seat).CanMutate() {
		return crmcontracts.AuthorizationSeatTypeFull
	}
	return crmcontracts.AuthorizationSeatTypeRead
}

// contractObjectGrants maps the merged permissions onto the wire shape.
//
// The field-by-field mapping is deliberate: principal.ObjectGrant carries no
// JSON tags, so handing it to the encoder would emit Create/Read/Update/Delete
// and every client check — which asks for the lowercase names the contract
// declares — would read absent and silently deny. That failure looks exactly
// like a correctly withheld permission, so it is worth the explicit copy.
func contractObjectGrants(objects map[string]principal.ObjectGrant) map[string]crmcontracts.RbacObjectGrant {
	grants := make(map[string]crmcontracts.RbacObjectGrant, len(objects))
	for object, grant := range objects {
		grants[object] = crmcontracts.RbacObjectGrant{
			Create: grant.Create,
			Read:   grant.Read,
			Update: grant.Update,
			Delete: grant.Delete,
		}
	}
	return grants
}

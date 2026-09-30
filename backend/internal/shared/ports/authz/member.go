// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package authz

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// MemberPrincipal is one member's live authority as a human principal acting
// for themselves: grants, teams and seat, read in one snapshot. It goes through
// AdmittedAuthority with no passport, because that is the read that takes all
// three together; reading the seat apart from the grants can compose an
// authority the member never held.
//
// The id is the "human:<uuid>" form the session door mints. A caller that
// records another form, or acts on somebody's behalf, sets that on the result;
// a member who is gone answers ErrNotFound, and what that means is the
// caller's to decide.
func MemberPrincipal(ctx context.Context, r Resolver, workspaceID, userID ids.UUID) (principal.Principal, error) {
	rbac, seat, err := r.AdmittedAuthority(ctx, workspaceID, userID, ids.Nil)
	if err != nil {
		return principal.Principal{}, err
	}
	return HumanPrincipal(userID, rbac, seat), nil
}

// HumanPrincipal is the principal a member's resolved authority makes, for a
// caller that resolved many members in one read rather than through
// MemberPrincipal one at a time.
func HumanPrincipal(userID ids.UUID, rbac RBAC, seat principal.SeatType) principal.Principal {
	return principal.Principal{
		Type:        principal.PrincipalHuman,
		ID:          "human:" + userID.String(),
		UserID:      userID,
		TeamIDs:     rbac.TeamIDs,
		SeatType:    seat,
		Permissions: rbac.Permissions,
	}
}

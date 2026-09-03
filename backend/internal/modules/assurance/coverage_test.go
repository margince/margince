// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seatWith builds a caller holding exactly the objects named.
func seatWith(objects map[string]principal.ObjectGrant) context.Context {
	user := ids.NewV7()
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"seat"}, Objects: objects,
			RowScope: principal.RowScopeAll,
		},
	})
}

// Source health is an operator's view, and reading the forecast does not buy
// it.
//
// The two are separate objects on purpose: a rep or a manager shown their
// installation's connector health has been handed somebody else's job, and a
// screen reporting a problem they cannot act on teaches them to ignore the
// screen. A gate on `forecast` would have given it to every seat that reads a
// number.
func TestReadingTheForecastDoesNotBuySourceHealth(t *testing.T) {
	t.Parallel()

	manager := seatWith(map[string]principal.ObjectGrant{
		"forecast": {Read: true, Create: true},
	})
	if err := auth.Require(manager, "data_coverage", principal.ActionRead); err == nil {
		t.Error("a seat holding forecast:read was admitted to source health — the two " +
			"are different jobs, and one screen reports a problem the other cannot act on")
	} else if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("the refusal was %v, want a permission denial", err)
	}

	// The admitting half. Without it a gate refusing EVERY caller would pass
	// the assertion above while the screen sat unreachable for its own
	// audience.
	operator := seatWith(map[string]principal.ObjectGrant{
		"forecast":      {Read: true},
		"data_coverage": {Read: true},
	})
	if err := auth.Require(operator, "data_coverage", principal.ActionRead); err != nil {
		t.Errorf("an operator holding data_coverage:read was refused: %v", err)
	}
}

// Nobody writes coverage. It is OBSERVED — what the nightly run could reach —
// and a grant for a verb the product does not offer reads as an oversight the
// next author has to research.
func TestSourceHealthHasNoWriteSurface(t *testing.T) {
	t.Parallel()

	operator := seatWith(map[string]principal.ObjectGrant{
		"data_coverage": {Read: true},
	})
	for _, action := range []principal.Action{
		principal.ActionCreate, principal.ActionUpdate, principal.ActionDelete,
	} {
		if err := auth.Require(operator, "data_coverage", action); err == nil {
			t.Errorf("%s was admitted on data_coverage — coverage is observed, and there "+
				"is nothing on this surface for anybody to write", action)
		}
	}
}

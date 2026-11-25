// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The store here has no database handle: a refusal or an empty answer that
// opened a transaction would fail with the handle's error instead.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func dealNamer(objects map[string]principal.ObjectGrant) context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:test", UserID: ids.NewV7(),
		Permissions: principal.Permissions{Objects: objects, RowScope: principal.RowScopeAll},
	})
}

func TestDealNamesRefuseACallerWithoutTheDealGrant(t *testing.T) {
	denied := dealNamer(map[string]principal.ObjectGrant{"contact": {Read: true}})
	want := []ids.UUID{ids.NewV7()}
	if _, err := (&Store{}).DealLabels(denied, want); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("DealLabels(no deal grant) = %v, want ErrPermissionDenied", err)
	}
	if _, err := (&Store{}).DealNamesTx(denied, nil, want); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("DealNamesTx(no deal grant) = %v, want ErrPermissionDenied", err)
	}
}

func TestDealLabelsOfNoDealsOpensNoTransaction(t *testing.T) {
	reader := dealNamer(map[string]principal.ObjectGrant{maintenanceObject: {Read: true}})
	labels, err := (&Store{}).DealLabels(reader, nil)
	if err != nil || len(labels) != 0 {
		t.Errorf("DealLabels(no ids) = %v, %v; want an empty answer and no error", labels, err)
	}
}

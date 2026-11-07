// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seatReading binds a rep who may or may not read activities. The activity
// grant is the object half of both source gates, and it is decided before
// either one reaches the database.
func seatReading(readsActivity bool) context.Context {
	objects := map[string]principal.ObjectGrant{"contact": {Create: true, Read: true}}
	if readsActivity {
		objects["activity"] = principal.ObjectGrant{Read: true}
	}
	user := ids.NewV7()
	return principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + user.String(), UserID: user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"}, Objects: objects, RowScope: principal.RowScopeOwn,
		},
	})
}

func TestACreateNamingNoSourceIsNotProbed(t *testing.T) {
	if err := refuseUnreadableSource(seatReading(false), nil, nil); err != nil {
		t.Errorf("a create with no source = %v, want it admitted: the probe has nothing to check", err)
	}
}

func TestACreateNamingASourceWithoutTheActivityGrantIsNotFound(t *testing.T) {
	source := ids.NewV7()
	err := refuseUnreadableSource(seatReading(false), nil, &source)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("create = %v, want not found: a 403 would say the record exists", err)
	}
}

func TestTheSourceColumnIsNullWithoutTheActivityGrant(t *testing.T) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }

	column, err := createdFromSQL(seatReading(false), arg)
	if err != nil {
		t.Fatal(err)
	}
	if column != `NULL::uuid` || len(args) != 0 {
		t.Errorf("column = %q with %d args, want a bare null: the reader may not open any activity", column, len(args))
	}
}

func TestTheSourceColumnReadsThroughTheReadersAudience(t *testing.T) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }

	column, err := createdFromSQL(seatReading(true), arg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(column, "contact.created_from_activity_id") {
		t.Errorf("column = %q, want the stored source read through a gated subselect", column)
	}
	if len(args) == 0 {
		t.Error("the audience binds the reader, so the column must carry at least one argument")
	}
}

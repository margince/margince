// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// An assignment body that left out user_id decodes to the zero UUID. The store
// refuses it by name before it opens a transaction. So the caller hears which
// field is missing, not a not-found for a user they never named.
func TestEveryRequiredLegalEntityBodyIDIsNamedWhenAbsent(t *testing.T) {
	ctx := principal.WithActor(principal.WithCorrelationID(context.Background(), ids.NewV7()), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String(), UserID: ids.NewV7(),
		Permissions: principal.Permissions{RoleKeys: []string{"admin"}, Objects: map[string]principal.ObjectGrant{
			legalEntityObject: {Read: true, Update: true}, userAdminObject: {Read: true, Update: true},
		}, RowScope: principal.RowScopeAll},
	})
	err := (&Store{}).AssignLegalEntityMember(ctx, ids.From[ids.LegalEntityKind](ids.NewV7()), ids.UserID{})
	var refusal *httperr.DetailedError
	if !errors.As(err, &refusal) || len(refusal.Fields) != 1 || refusal.Fields[0].Field != "user_id" {
		t.Fatalf("an assignment with no user_id answered %v, want a refusal naming user_id", err)
	}
}

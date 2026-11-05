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

// A link body that left out company_id decodes to the zero UUID. The store
// refuses it by name before it opens a transaction. The caller then hears
// which field is missing, not a not-found for a company they never named.
func TestEveryRequiredLeadCompanyBodyIDIsNamedWhenAbsent(t *testing.T) {
	ctx := principal.WithActor(principal.WithCorrelationID(context.Background(), ids.NewV7()), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String(), UserID: ids.NewV7(),
		Permissions: principal.Permissions{RoleKeys: []string{"rep"}, Objects: map[string]principal.ObjectGrant{
			"lead": {Update: true}, "company": {Read: true},
		}, RowScope: principal.RowScopeAll},
	})
	_, err := (&Store{}).LinkLeadCompany(ctx, ids.From[ids.LeadKind](ids.NewV7()), ids.CompanyID{})
	var refusal *httperr.DetailedError
	if !errors.As(err, &refusal) || len(refusal.Fields) != 1 || refusal.Fields[0].Field != "company_id" {
		t.Fatalf("a link with no company_id answered %v, want a refusal naming company_id", err)
	}
}

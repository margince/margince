// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestEveryRequiredShareBodyIDIsNamedWhenAbsent(t *testing.T) {
	_, err := ShareCompanyContacts(context.Background(), nil, identity.NewService(nil), ids.CompanyID{}, crmcontracts.ShareCompanyContactsRequest{})
	refused, ok := errors.AsType[*httperr.DetailedError](err)
	if !ok || len(refused.Fields) != 1 || refused.Fields[0].Field != "subject_id" || refused.Fields[0].Code != "required" {
		t.Fatalf("an omitted subject_id returned %v, want the required-field refusal", err)
	}
}

func TestANamedShareRecipientReachesTheAuthorityCheck(t *testing.T) {
	ctx := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, ID: "human:test"})
	_, err := ShareCompanyContacts(ctx, nil, identity.NewService(nil), ids.CompanyID{}, crmcontracts.ShareCompanyContactsRequest{
		SubjectType: "user", SubjectId: crmcontracts.Id(ids.NewV7()), Access: "read",
	})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a named recipient returned %v, want the authority refusal", err)
	}
}

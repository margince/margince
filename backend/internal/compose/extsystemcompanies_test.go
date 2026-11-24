// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

// Each new read needs its own declaration, runs only where no user gets the
// answer, and refuses a malformed id before it touches the database.
func TestTheSeatReadsNeedTheirDeclarationAnUnattendedCallAndAnActAs(t *testing.T) {
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	binding := extensionRuntimeBinding{pool: &pgxpool.Pool{}}
	company := ids.NewV7().String()
	seat := extension.UserID(ids.NewV7().String())

	composeReader(t, extension.SystemReadDeals)
	undeclared := deliveryRuntimeFor(ctx, "dealmirror", "1.0.0", "subscription/mirror", binding)
	defer undeclared.release()
	if _, err := undeclared.Companies().Get(ctx, seat, company); !errors.Is(err, extension.ErrUndeclaredRead) {
		t.Errorf("an undeclared Companies().Get: err = %v, want ErrUndeclaredRead", err)
	}
	if _, err := undeclared.MailContent().Get(ctx, seat, company); !errors.Is(err, extension.ErrUndeclaredRead) {
		t.Errorf("an undeclared MailContent().Get: err = %v, want ErrUndeclaredRead", err)
	}

	composeReader(t, extension.SystemReadDeals)
	if _, err := undeclared.Directory().ByRole(ctx, seat, []string{"reviewer"}); !errors.Is(err, extension.ErrUndeclaredRead) {
		t.Errorf("an undeclared Directory().ByRole: err = %v, want ErrUndeclaredRead", err)
	}

	composeReader(t, extension.SystemReadCompanies, extension.SystemReadMailContent, extension.SystemReadDirectory)
	attended := runtimeFor(ctx, "dealmirror", "1.0.0", "tool/probe", binding)
	defer attended.release()
	if _, err := attended.Companies().List(ctx, seat, extension.CompanyPageQuery{}); !errors.Is(err, extension.ErrAttendedRead) {
		t.Errorf("Companies().List on a signed-in call: err = %v, want ErrAttendedRead", err)
	}
	if _, err := attended.MailContent().Get(ctx, seat, company); !errors.Is(err, extension.ErrAttendedRead) {
		t.Errorf("MailContent().Get on a signed-in call: err = %v, want ErrAttendedRead", err)
	}

	declared := deliveryRuntimeFor(ctx, "dealmirror", "1.0.0", "subscription/mirror", binding)
	defer declared.release()
	if _, err := attended.Directory().ByRole(ctx, seat, nil); !errors.Is(err, extension.ErrAttendedRead) {
		t.Errorf("Directory().ByRole on a signed-in call: err = %v, want ErrAttendedRead", err)
	}
	// No actAs is refused before the database is asked, by every seat read.
	if _, err := declared.Companies().Get(ctx, "", company); !errors.Is(err, extension.ErrInvalid) {
		t.Errorf("Companies().Get with no actAs: err = %v, want ErrInvalid", err)
	}
	if _, err := declared.Companies().List(ctx, "", extension.CompanyPageQuery{}); !errors.Is(err, extension.ErrInvalid) {
		t.Errorf("Companies().List with no actAs: err = %v, want ErrInvalid", err)
	}
	if _, err := declared.MailContent().Get(ctx, "", company); !errors.Is(err, extension.ErrInvalid) {
		t.Errorf("MailContent().Get with no actAs: err = %v, want ErrInvalid", err)
	}
	if _, err := declared.Directory().ByRole(ctx, "not-an-id", nil); !errors.Is(err, extension.ErrInvalid) {
		t.Errorf("Directory().ByRole with a malformed actAs: err = %v, want ErrInvalid", err)
	}
}

// A field the seat's mask withholds on a company is left empty in the summary.
func TestTheCompanySummaryEmptiesWhatTheSeatsMaskWithholds(t *testing.T) {
	seat := principal.Principal{
		Type: principal.PrincipalHuman, UserID: ids.NewV7(),
		Permissions: principal.Permissions{
			RowScope:   principal.RowScopeOwn,
			FieldMasks: []principal.FieldMask{{Object: "company", Field: "legal_name", Condition: principal.MaskAlways}},
		},
	}
	legal := "Shown AG"
	company := crmcontracts.Company{Id: openapi_types.UUID(ids.NewV7()), DisplayName: "Shown", LegalName: &legal}
	got := summarizeCompany(principal.WithActor(context.Background(), seat), company, outsideID{system: "erp", id: "1"})
	if got.Name != "Shown" || got.SourceID != "1" || got.LegalName != "" {
		t.Errorf("summary = %+v, want the name and outside id and no legal name", got)
	}
}

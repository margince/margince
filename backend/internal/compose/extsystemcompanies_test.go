// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

// Each new read needs its own declaration, runs only where no user gets the
// answer, and refuses a malformed id before it touches the database.
func TestTheCompanyAndMailContentReadsNeedTheirDeclarationAndAnUnattendedCall(t *testing.T) {
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	binding := extensionRuntimeBinding{pool: &pgxpool.Pool{}}
	company := ids.NewV7().String()

	composeReader(t, extension.SystemReadDeals)
	undeclared := deliveryRuntimeFor(ctx, "dealmirror", "1.0.0", "subscription/mirror", binding)
	defer undeclared.release()
	if _, err := undeclared.Companies().Get(ctx, company); !errors.Is(err, extension.ErrUndeclaredRead) {
		t.Errorf("an undeclared Companies().Get: err = %v, want ErrUndeclaredRead", err)
	}
	if _, err := undeclared.MailContent().Get(ctx, company); !errors.Is(err, extension.ErrUndeclaredRead) {
		t.Errorf("an undeclared MailContent().Get: err = %v, want ErrUndeclaredRead", err)
	}

	composeReader(t, extension.SystemReadCompanies, extension.SystemReadMailContent)
	attended := runtimeFor(ctx, "dealmirror", "1.0.0", "tool/probe", binding)
	defer attended.release()
	if _, err := attended.Companies().List(ctx, extension.CompanyPageQuery{}); !errors.Is(err, extension.ErrAttendedRead) {
		t.Errorf("Companies().List on a signed-in call: err = %v, want ErrAttendedRead", err)
	}
	if _, err := attended.MailContent().Get(ctx, company); !errors.Is(err, extension.ErrAttendedRead) {
		t.Errorf("MailContent().Get on a signed-in call: err = %v, want ErrAttendedRead", err)
	}

	declared := deliveryRuntimeFor(ctx, "dealmirror", "1.0.0", "subscription/mirror", binding)
	defer declared.release()
	if _, err := declared.Companies().Get(ctx, "not-an-id"); !errors.Is(err, extension.ErrInvalid) {
		t.Errorf("Companies().Get with a malformed id: err = %v, want ErrInvalid", err)
	}
	if _, err := declared.Companies().List(ctx, extension.CompanyPageQuery{Cursor: "x"}); !errors.Is(err, extension.ErrInvalid) {
		t.Errorf("Companies().List with a bad cursor: err = %v, want ErrInvalid", err)
	}
	if _, err := declared.MailContent().Get(ctx, "not-an-id"); !errors.Is(err, extension.ErrInvalid) {
		t.Errorf("MailContent().Get with a malformed id: err = %v, want ErrInvalid", err)
	}
}

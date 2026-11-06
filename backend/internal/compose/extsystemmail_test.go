// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

// A unit reads a deal's mail only when it declares the group, and only where
// no user gets the answer.
func TestTheMailReadNeedsItsDeclarationAndAnUnattendedCall(t *testing.T) {
	deal := ids.NewV7().String()
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	binding := extensionRuntimeBinding{pool: &pgxpool.Pool{}}

	composeReader(t, extension.SystemReadDeals)
	undeclared := deliveryRuntimeFor(ctx, "dealmirror", "1.0.0", "subscription/mirror", binding)
	defer undeclared.release()
	if _, err := undeclared.Activities().Mail(ctx, deal, extension.MailQuery{}); !errors.Is(err, extension.ErrUndeclaredRead) {
		t.Errorf("an undeclared Mail: err = %v, want ErrUndeclaredRead", err)
	}

	composeReader(t, extension.SystemReadActivities)
	attended := runtimeFor(ctx, "dealmirror", "1.0.0", "tool/probe", binding)
	defer attended.release()
	if _, err := attended.Activities().Mail(ctx, deal, extension.MailQuery{}); !errors.Is(err, extension.ErrAttendedRead) {
		t.Errorf("Mail on a signed-in call: err = %v, want ErrAttendedRead", err)
	}
	declared := deliveryRuntimeFor(ctx, "dealmirror", "1.0.0", "subscription/mirror", binding)
	defer declared.release()
	if _, err := declared.Activities().Mail(ctx, "not-an-id", extension.MailQuery{}); !errors.Is(err, extension.ErrInvalid) {
		t.Errorf("Mail with a malformed deal id: err = %v, want ErrInvalid", err)
	}
}

func TestAMailCursorOnlyReadsBackWhatTheReadGave(t *testing.T) {
	at := mailPosition{at: clocktest.Now(t).UTC().Truncate(time.Microsecond), id: ids.NewV7()}
	back, err := parseMailCursor(at.cursor())
	if err != nil || back == nil || !back.at.Equal(at.at) || back.id != at.id {
		t.Fatalf("a cursor read back as %+v, %v; want %+v", back, err, at)
	}
	for _, bad := range []string{"!!", "bm90LWEtY3Vyc29y"} {
		if _, err := parseMailCursor(bad); !errors.Is(err, extension.ErrInvalid) {
			t.Errorf("cursor %q: err = %v, want ErrInvalid", bad, err)
		}
	}
}

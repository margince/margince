// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A unit files its own signal kind through the core port, on the transaction
// it holds and under the caller's rights. The same event reported twice is one
// signal, and the second report is a read: it is held to the caller's row scope.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/signaltypes"
	"github.com/margince/margince/backend/pkg/extension"
)

func signalFiler(objects ...string) principal.Permissions {
	grants := map[string]principal.ObjectGrant{}
	for _, o := range objects {
		grants[o] = principal.ObjectGrant{Create: true, Read: true}
	}
	return principal.Permissions{RoleKeys: []string{"filer"}, Objects: grants, RowScope: principal.RowScopeOwn}
}

func TestAUnitFilesItsOwnSignalKindAndARepeatIsTheSameSignal(t *testing.T) {
	w := setupReads(t)
	signaltypes.Register(coreUnit, []extension.SignalTypeDeclaration{{
		Kind: "loss", Label: "Loss reported",
		Evidence: []extension.SignalEvidenceField{{Name: "amount", Label: "Amount", Type: extension.SignalFieldNumber, Required: true}},
	}})
	subject := extension.SignalSubject{Type: "company", ID: w.mineCompany.String()}
	evidence := extension.SignalEvidence{Fields: map[string]any{"amount": 40.0}}
	perms := signalFiler("signal", "company")

	var firstID string
	err := w.readAs(t, w.user, perms, func(ctx context.Context, core extension.Core) error {
		first, created, err := core.Signals().Create(ctx, "loss", subject, evidence, "feed-event-1")
		if err != nil || !created || string(first.Kind) != "ext:notes:loss" {
			t.Fatalf("first report = %v created=%v err=%v, want a new ext:notes:loss signal", first.Kind, created, err)
		}
		firstID = first.Id
		again, created, err := core.Signals().Create(ctx, "loss", subject, evidence, "feed-event-1")
		if err != nil || created || again.Id != firstID {
			t.Errorf("repeat report = %v created=%v err=%v, want the first signal back and nothing new", again.Id, created, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// A second reporter, a colleague at a seat, reports the same event.
	err = w.readAs(t, w.user, perms, func(ctx context.Context, core extension.Core) error {
		colleague, created, err := core.Signals().Create(ctx, "loss", subject, evidence, "feed-event-1")
		if err != nil || created || colleague.Id != firstID {
			t.Errorf("colleague report = %v created=%v err=%v, want the feed's signal", colleague.Id, created, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAUnitCannotFileASignalAboutARecordTheCallerCannotSee(t *testing.T) {
	w := setupReads(t)
	signaltypes.Register(coreUnit, []extension.SignalTypeDeclaration{{Kind: "loss", Label: "Loss reported"}})
	err := w.readAs(t, w.user, signalFiler("signal", "company"), func(ctx context.Context, core extension.Core) error {
		_, _, err := core.Signals().Create(ctx, "loss",
			extension.SignalSubject{Type: "company", ID: w.theirsCompany.String()}, extension.SignalEvidence{}, "hidden-1")
		if !errors.Is(err, extension.ErrNotFound) {
			t.Errorf("a signal about another seat's private company answered %v, want ErrNotFound", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = w.readAs(t, w.user, signalFiler("company"), func(ctx context.Context, core extension.Core) error {
		_, _, err := core.Signals().Create(ctx, "loss",
			extension.SignalSubject{Type: "company", ID: w.mineCompany.String()}, extension.SignalEvidence{}, "no-right-1")
		if !errors.Is(err, extension.ErrForbidden) {
			t.Errorf("a caller without the signal create right answered %v, want ErrForbidden", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

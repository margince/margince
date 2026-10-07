// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A seat that may not delete activities is refused the same way whether it asks
// what a purge would destroy or confirms it.
func TestAPurgePreviewRefusesWhereTheConfirmWould(t *testing.T) {
	e := integration.Setup(t)
	mail := seedPurgeableMail(t, e, "anwalt@kanzlei.example", "meine", e.Rep1)
	rule := seedOwnExclusion(t, e, e.Rep1, capture.ExclusionKindDomain, "kanzlei.example")

	noDelete := principal.Permissions{
		Objects: map[string]principal.ObjectGrant{
			"activity": {Read: true, Create: true, Update: true},
			"contact":  {Read: true, Create: true, Update: true},
		},
		RowScope: principal.RowScopeAll,
	}
	ask := func(preview bool) error {
		ctx := purgeCtx(e, e.Rep1)
		actor, ok := principal.Actor(ctx)
		if !ok {
			t.Fatal("purgeCtx carries no actor")
		}
		actor.Permissions = noDelete
		_, err := purgerFor(t, e).Purge(principal.WithActor(ctx, actor), rule, preview)
		return err
	}

	if err := ask(false); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("the confirm answered %v, want permission denied", err)
	}
	if err := ask(true); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("the preview answered %v, want the same refusal as the confirm", err)
	}
	// The refusal came before anything ran, which is what the dialog tells the
	// reader.
	if body := activityBody(t, e, mail); body == "" {
		t.Error("a refused purge destroyed the message anyway")
	}
}

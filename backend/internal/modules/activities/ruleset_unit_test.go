// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Every door that reads or writes a prompt digest refuses an empty one before
// it reaches the database. The store below has no pool, so a door that let the
// empty digest through would fail on the nil database instead — with some other
// error, which errors.Is tells apart.
func TestEveryDigestDoorRefusesAnEmptyDigest(t *testing.T) {
	store := NewStore(nil)
	ctx := principal.WithActor(principal.WithWorkspaceID(context.Background(), ids.NewV7()), principal.Principal{
		Type: principal.PrincipalSystem, ID: OwedVerdictCapturedBy,
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
	id := ids.NewV7()
	for door, call := range map[string]func() error{
		"MarkCaptureLabelDeclined": func() error { _, err := store.MarkCaptureLabelDeclined(ctx, id, ""); return err },
		"MarkOwedVerdictDeclined":  func() error { _, err := store.MarkOwedVerdictDeclined(ctx, id, ""); return err },
		"UnlabeledCaptureEmails":   func() error { _, err := store.UnlabeledCaptureEmails(ctx, "", 1, 1); return err },
		"ReadPipelineFacts":        func() error { _, err := store.ReadPipelineFacts(ctx, id, ""); return err },
		"OwedBacklog":              func() error { _, _, err := store.OwedBacklog(ctx, "", time.Now(), 1, 1, 1); return err },
		"OwedRestale":              func() error { _, _, err := store.OwedRestale(ctx, "", 1, 1, 1); return err },
		"SetOwedVerdict": func() error {
			_, err := store.SetOwedVerdict(ctx, id, OwedVerdictAsksUs, "", time.Now())
			return err
		},
	} {
		if err := call(); !errors.Is(err, errRulesetUnnamed) {
			t.Errorf("%s with no digest: err = %v, want errRulesetUnnamed", door, err)
		}
	}
}

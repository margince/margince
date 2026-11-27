// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"errors"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// An archived purpose keeps its key, and the refusal says so. The admin is told to
// restore that purpose. A bare "already exists" would name a purpose the list does not show.
func TestAnArchivedPurposeKeepsItsKeyAndTheRefusalSaysSo(t *testing.T) {
	t.Parallel()
	e := setupChannelConsent(t)
	configurer := principal.WithActor(e.ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.user.String(), UserID: e.user,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"consent_config": {Create: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	created, err := e.store.CreatePurpose(configurer, "quarterly_digest", "Quarterly digest", false, "")
	if err != nil {
		t.Fatalf("creating the purpose: %v", err)
	}

	_, err = e.store.CreatePurpose(configurer, "quarterly_digest", "Another digest", false, "")
	if !errors.Is(err, apperrors.ErrConflict) || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("a live key was refused with %v, want a conflict saying the key already exists", err)
	}

	if _, err := e.owner.Exec(e.ctx, `UPDATE consent_purpose SET archived_at = now() WHERE id = $1`, created.ID); err != nil {
		t.Fatalf("archiving the purpose: %v", err)
	}
	_, err = e.store.CreatePurpose(configurer, "quarterly_digest", "Another digest", false, "")
	if !errors.Is(err, apperrors.ErrConflict) || !strings.Contains(err.Error(), "archived purpose") {
		t.Fatalf("an archived key was refused with %v, want a conflict naming the archived purpose", err)
	}
}

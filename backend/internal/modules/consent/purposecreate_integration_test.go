// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A new purpose's key holds no space, and its label cannot read as a listed
// purpose's label or key: the consent screens name a purpose by its label alone.
func TestANewPurposeCannotBeMistakenForAListedOne(t *testing.T) {
	e := setupChannelConsent(t)
	configurer := principal.WithActor(e.ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.user.String(), UserID: e.user,
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"consent_config": {Create: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	listed, err := e.store.CreatePurpose(configurer, "partner_news", "Partner news", false, "")
	if err != nil {
		t.Fatalf("creating the listed purpose: %v", err)
	}

	refusals := []struct {
		name, key, label, field string
	}{
		{name: "a key with a space", key: "partner news", label: "Partner digest", field: "key"},
		{name: "a key with a hyphen", key: "partner-digest", label: "Partner digest", field: "key"},
		{name: "a label that is a listed key", key: "partner_digest", label: "partner_news", field: "label"},
		{name: "a label that is a listed label, case and spacing aside", key: "partner_digest", label: "  PARTNER   news ", field: "label"},
	}
	for _, tc := range refusals {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.store.CreatePurpose(configurer, tc.key, tc.label, false, "")
			var refused *ValidationError
			if !errors.As(err, &refused) || refused.Field != tc.field {
				t.Fatalf("CreatePurpose(%q, %q) = %v, want a refusal on %s", tc.key, tc.label, err, tc.field)
			}
		})
	}

	if _, err := e.owner.Exec(e.ctx, `UPDATE consent_purpose SET archived_at = now() WHERE id = $1`, listed.ID); err != nil {
		t.Fatalf("archiving the listed purpose: %v", err)
	}
	created, err := e.store.CreatePurpose(configurer, " Partner_Digest ", " Partner news ", false, "")
	if err != nil {
		t.Fatalf("a label only an archived purpose holds was refused: %v", err)
	}
	if created.Key != "partner_digest" || created.Label != "Partner news" {
		t.Fatalf("stored key %q and label %q, want them trimmed and the key lower-cased", created.Key, created.Label)
	}
}

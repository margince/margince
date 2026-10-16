// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"errors"
	"io/fs"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// contactsConfig is this module's configuration as a program loads it from
// environ alone, reading and saving through pool.
func contactsConfig(t *testing.T, pool *pgxpool.Pool, environ ...string) *config.Config {
	t.Helper()
	reg, err := config.NewRegistry(ConfigSettings())
	if err != nil {
		t.Fatalf("the contacts settings do not register: %v", err)
	}
	cfg, err := config.Load(reg, config.Sources{
		Environ:  environ,
		ReadFile: func(string) ([]byte, error) { return nil, fs.ErrNotExist },
	})
	if err != nil {
		t.Fatalf("loading the contacts settings from %v: %v", environ, err)
	}
	return cfg.WithDatabase(settings.New(pool, settings.NewRegistry(cfg.StoredDefinitions()...)))
}

// bindForTest binds cfg as the program's configuration until the test ends.
func bindForTest(t *testing.T, cfg *config.Config) {
	t.Helper()
	before := config.Bound()
	config.Bind(cfg)
	t.Cleanup(func() { config.Bind(before) })
}

// A target the environment sets pairs with the switch an admin saved, so a
// lead read applies both. Not parallel: the binding is the process's own.
func TestABootSetTargetPairsWithTheSavedSwitch(t *testing.T) {
	e := setupPromoteConsent(t)
	pool := e.store.db.Pool()
	on, saved := true, 30
	if _, err := e.store.WithSettings(contactsConfig(t, pool)).UpdateLeadSettings(e.ctx,
		UpdateLeadSettingsInput{FirstResponseEnabled: &on, FirstResponseTargetMinutes: &saved}); err != nil {
		t.Fatalf("saving the policy with nothing set at boot: %v", err)
	}
	locked := contactsConfig(t, pool, "MARGINCE_CONTACTS_FIRST_RESPONSE_TARGET_MINUTES=90")
	bindForTest(t, locked)

	got, err := e.store.WithSettings(locked).GetLeadSettings(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !got.FirstResponseEnabled || got.FirstResponseTargetMinutes != 90 {
		t.Fatalf("the policy reads %+v, want the saved switch on and the 90 minutes the environment sets", got)
	}
}

// A seat the environment sets is the one a breach escalates to, and a change
// to any other seat is refused as locked before that seat is judged.
func TestABootSetEscalationSeatRefusesAnotherBeforeJudgingIt(t *testing.T) {
	e := setupPromoteConsent(t)
	locked := contactsConfig(t, e.store.db.Pool(), "MARGINCE_CONTACTS_UNASSIGNED_ESCALATION_USER_ID="+e.user.String())
	bindForTest(t, locked)
	store := e.store.WithSettings(locked)

	got, err := store.GetLeadSettings(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.UnassignedEscalationUserId == nil || ids.UUID(*got.UnassignedEscalationUserId) != e.user {
		t.Fatalf("the escalation seat reads %v, want the %s the environment sets", got.UnassignedEscalationUserId, e.user)
	}
	nobody := ids.From[ids.UserKind](ids.NewV7())
	if _, err := store.UpdateLeadSettings(e.ctx, UpdateLeadSettingsInput{UnassignedEscalationUserID: &nobody}); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("naming a seat that is no colleague under a boot-set seat = %v, want a conflict", err)
	}
	same := ids.From[ids.UserKind](e.user)
	if _, err := store.UpdateLeadSettings(e.ctx, UpdateLeadSettingsInput{UnassignedEscalationUserID: &same}); err != nil {
		t.Errorf("resending the boot-set seat: %v", err)
	}
}

// A signature template the environment sets is the one a send signs with,
// over the row an admin saved.
func TestABootSetTemplateIsTheOneASendSignsWith(t *testing.T) {
	e := setupPromoteConsent(t)
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO setting (key, value, updated_at) VALUES ($1, '"<p>saved</p>"'::jsonb, now())`,
		SignatureTemplate.Key()); err != nil {
		t.Fatal(err)
	}
	bindForTest(t, contactsConfig(t, e.store.db.Pool(), "MARGINCE_CONTACTS_SIGNATURE_TEMPLATE=<p>{name}</p>"))

	signature, err := e.store.SignatureFor(e.ctx, e.user)
	if err != nil {
		t.Fatal(err)
	}
	if signature.Template != "<p>{name}</p>" {
		t.Fatalf("a send signs with %q, want the template the environment sets", signature.Template)
	}
}

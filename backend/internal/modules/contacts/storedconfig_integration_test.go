// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config/configtest"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A target the environment sets pairs with the switch an admin saved. The
// settings surface reads it through the configuration its writes lock against.
func TestABootSetTargetPairsWithTheSavedSwitch(t *testing.T) {
	e := setupPromoteConsent(t)
	pool := e.store.db.Pool()
	on, saved := true, 30
	if _, err := e.store.WithSettings(configtest.StoredOver(t, pool, ConfigSettings())).UpdateLeadSettings(e.ctx,
		UpdateLeadSettingsInput{FirstResponseEnabled: &on, FirstResponseTargetMinutes: &saved}); err != nil {
		t.Fatalf("saving the policy with nothing set at boot: %v", err)
	}
	locked := configtest.StoredOver(t, pool, ConfigSettings(), "MARGINCE_CONTACTS_FIRST_RESPONSE_TARGET_MINUTES=90")

	got, err := e.store.WithSettings(locked).GetLeadSettings(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !got.FirstResponseEnabled || got.FirstResponseTargetMinutes != 90 {
		t.Fatalf("the policy reads %+v, want the saved switch on and the 90 minutes the environment sets", got)
	}
}

// A seat the environment sets is the one a breach escalates to. A change to
// any other seat is refused as locked before that seat is judged.
func TestABootSetEscalationSeatRefusesAnotherBeforeJudgingIt(t *testing.T) {
	e := setupPromoteConsent(t)
	locked := configtest.StoredOver(t, e.store.db.Pool(), ConfigSettings(), "MARGINCE_CONTACTS_UNASSIGNED_ESCALATION_USER_ID="+e.user.String())
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
	configtest.Bind(t, configtest.StoredOver(t, e.store.db.Pool(), ConfigSettings(), "MARGINCE_CONTACTS_SIGNATURE_TEMPLATE=<p>{name}</p>"))

	signature, err := e.store.SignatureFor(e.ctx, e.user)
	if err != nil {
		t.Fatal(err)
	}
	if signature.Template != "<p>{name}</p>" {
		t.Fatalf("a send signs with %q, want the template the environment sets", signature.Template)
	}
}

// A boot-set seat no colleague holds escalates to nobody, and the log names
// the setting and the variable that set it. Not parallel: the log and the
// binding are the process's own.
func TestABootSetSeatNoColleagueHoldsIsWarnedAbout(t *testing.T) {
	e := setupPromoteConsent(t)
	locked := configtest.StoredOver(t, e.store.db.Pool(), ConfigSettings(),
		"MARGINCE_CONTACTS_UNASSIGNED_ESCALATION_USER_ID="+ids.NewV7().String())
	var logged bytes.Buffer
	restore := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(restore) })

	got, err := e.store.WithSettings(locked).GetLeadSettings(e.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.UnassignedEscalationUserId != nil {
		t.Fatalf("a seat no colleague holds reads %v, want nobody", got.UnassignedEscalationUserId)
	}
	for _, want := range []string{UnassignedEscalationUserID.Key(), "MARGINCE_CONTACTS_UNASSIGNED_ESCALATION_USER_ID"} {
		if !strings.Contains(logged.String(), want) {
			t.Errorf("the warning %q does not name %s", logged.String(), want)
		}
	}
}

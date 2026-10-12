// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The installation's jurisdiction under the retention floor. This binary
// carries two packs: zq with a six-calendar-year correspondence floor
// (retention_jurisdiction_integration_test.go), and zk, which declares none.

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
)

// noFloorPack is a jurisdiction that declares no correspondence floor.
type noFloorPack struct{}

func (noFloorPack) Code() jurisdiction.Code { return "zk" }

func (noFloorPack) Retention() jurisdiction.Retention { return nil }

func init() { jurisdiction.Register(noFloorPack{}) }

// floorFixture holds one won deal, so every message linked to it is a
// Handelsbrief the floor protects.
type floorFixture struct {
	e    *Env
	deal ids.UUID
}

func newFloorFixture(t *testing.T) floorFixture {
	t.Helper()
	e := Setup(t)
	f := floorFixture{e: e, deal: ids.NewV7()}
	pipeline, stage := ids.NewV7(), ids.NewV7()
	f.exec(t, `INSERT INTO pipeline (id, name, is_default, position) VALUES ($1, 'Default', true, 0)`, pipeline)
	f.exec(t, `INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability) VALUES ($1, $2, 'Won', 0, 'won', 100)`, stage, pipeline)
	f.exec(t, `INSERT INTO deal (id, name, status, pipeline_id, stage_id, closed_at, source, captured_by)
		VALUES ($1, 'Acme rollout', 'won', $2, $3, now(), 'api', 'human:t')`, f.deal, pipeline, stage)
	f.exec(t, `INSERT INTO retention_policy (object_type, category, retain_days, action) VALUES ('activity', NULL, 100, 'erase')`)
	return f
}

func (f floorFixture) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if err := database.WithWorkspaceTx(f.e.Admin(), f.e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), sql, args...)
		return err
	}); err != nil {
		t.Fatalf("%s: %v", strings.Fields(sql)[0], err)
	}
}

// mail seeds a Handelsbrief that occurred ageDays ago.
func (f floorFixture) mail(t *testing.T, ageDays int) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	f.exec(t, `INSERT INTO activity (id, kind, subject, body, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'Order', 'commercial content', now() - make_interval(days => $2), 'capture', 'connector:t')`, id, ageDays)
	f.exec(t, `INSERT INTO activity_link (activity_id, entity_type, deal_id) VALUES ($1, 'deal', $2)`, id, f.deal)
	return id
}

// governor may change the jurisdiction: the change writes the country and the
// carried floor in one transaction.
func (f floorFixture) governor() context.Context {
	perms := principal.Permissions{RoleKeys: AdminPerms.RoleKeys, RowScope: principal.RowScopeAll, Objects: maps.Clone(AdminPerms.Objects)}
	perms.Objects["installation_settings"] = principal.ObjectGrant{Read: true, Update: true}
	perms.Objects["retention_policy"] = principal.ObjectGrant{Read: true, Update: true}
	return f.e.As(f.e.AdminUser, nil, perms)
}

func (f floorFixture) setCountry(t *testing.T, code string) {
	t.Helper()
	if _, err := compose.NewInstallationSettingsFor(f.e.Pool).UpdateInstallation(f.governor(), identity.InstallationPatch{Country: &code}); err != nil {
		t.Fatalf("setting the jurisdiction to %q: %v", code, err)
	}
}

func (f floorFixture) retentionPass(t *testing.T) {
	t.Helper()
	SettleIntoInstall(t, f.e)
	svc := compose.NewRetentionServiceFor(f.e.DB(), nil, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	if err := svc.EvaluateInstallation(RetentionPassCtx(f.e.WS)); err != nil {
		t.Fatal(err)
	}
}

func (f floorFixture) erased(t *testing.T, id ids.UUID) bool {
	t.Helper()
	var body *string
	if err := database.WithWorkspaceTx(f.e.Admin(), f.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT body FROM activity WHERE id = $1`, id).Scan(&body)
	}); err != nil {
		t.Fatal(err)
	}
	return body == nil
}

func (f floorFixture) policy(t *testing.T) privacy.FloorPolicy {
	t.Helper()
	var policy privacy.FloorPolicy
	if err := database.WithWorkspaceTx(f.e.Admin(), f.e.Pool, func(tx pgx.Tx) error {
		var err error
		policy, err = privacy.ResolveFloorPolicy(context.Background(), tx, identity.CountryOf)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return policy
}

// No jurisdiction selected: the strictest compiled-in floor holds, so a pack
// declaring none does not unshield mail the other pack protects.
func TestAnUnsetJurisdictionKeepsTheStrictestCompiledInFloor(t *testing.T) {
	f := newFloorFixture(t)
	old := f.mail(t, 400)
	f.retentionPass(t)
	if f.erased(t, old) {
		t.Fatal("a 400-day Handelsbrief was erased with no jurisdiction selected — the zq floor no longer applies")
	}
}

// A jurisdiction selected at setup whose pack declares no floor applies none.
func TestASelectedPackWithNoFloorAppliesNone(t *testing.T) {
	f := newFloorFixture(t)
	f.exec(t, `INSERT INTO setting (key, value) VALUES ('installation.country', '"zk"')`)
	old := f.mail(t, 400)
	f.retentionPass(t)
	if !f.erased(t, old) {
		t.Fatal("a 400-day Handelsbrief survived under zk, which declares no floor — the selection is ignored")
	}
	posture := privacy.StatutoryFloorPosture(f.policy(t))
	if posture.Selected != "zk" || posture.Known || posture.Carried != nil {
		t.Fatalf("posture = %+v, want zk selected with no floor and nothing carried", posture)
	}
}

// Changing an existing installation from unset to zk weakens the floor. Mail
// that occurred before the change keeps the zq floor; mail after it has none.
func TestAWeakeningChangeCarriesTheOldFloorForOlderMail(t *testing.T) {
	f := newFloorFixture(t)
	f.setCountry(t, "zk")
	carried := f.policy(t).Carried
	if carried == nil || carried.Class.Keep != (jurisdiction.Period{Years: 6}) {
		t.Fatalf("carried = %+v, want the six-year zq floor carried", carried)
	}
	// Moved back 200 days, so mail "after the change" can be old enough for
	// the 100-day policy to reach it.
	f.exec(t, `UPDATE setting SET value = jsonb_set(value, '{before}', to_jsonb(now() - interval '200 days'))
		WHERE key = 'privacy.retention_floor_carry'`)
	before, after := f.mail(t, 400), f.mail(t, 150)
	f.retentionPass(t)
	if f.erased(t, before) {
		t.Error("mail from before the change was erased — the carried floor did not bind it")
	}
	if !f.erased(t, after) {
		t.Error("mail from after the change survived — the carried floor bound mail it never covered")
	}
}

// A controller's pin on old mail pins to the carried floor, never to the
// current one that has already let it go.
func TestAPinOnOldMailPinsToTheCarriedFloor(t *testing.T) {
	f := newFloorFixture(t)
	f.setCountry(t, "zk")
	old := f.mail(t, 400)
	reason, err := privacy.ParseStatedReason("supplier correspondence the derivation cannot see")
	if err != nil {
		t.Fatal(err)
	}
	controller := controllerCtx(f.e, principal.ObjectGrant{Read: true, Update: true})
	if err := compose.ComposedEraser(f.e.Pool, nil, nil).PinToFloor(controller, old, reason); err != nil {
		t.Fatalf("PinToFloor: %v", err)
	}
	var until time.Time
	if err := database.WithWorkspaceTx(f.e.Admin(), f.e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT restricted_until FROM activity WHERE id = $1`, old).Scan(&until)
	}); err != nil {
		t.Fatal(err)
	}
	if until.Before(time.Now().AddDate(4, 0, 0)) {
		t.Fatalf("restricted_until = %s, want the end of the six-year carried window", until)
	}
}

// A purge selected while nothing protected the mail and executed after a
// change to a jurisdiction that does must not destroy it.
func TestAPurgeExecutedAfterAStricterChangeKeepsTheMail(t *testing.T) {
	f := newFloorFixture(t)
	f.setCountry(t, "zk")
	f.exec(t, `UPDATE setting SET value = jsonb_set(value, '{before}', to_jsonb(now() - interval '1000 days'))
		WHERE key = 'privacy.retention_floor_carry'`)
	selected := f.mail(t, 150)
	f.setCountry(t, "zq")
	svc := compose.NewRetentionServiceFor(f.e.DB(), nil, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	destroyed, err := svc.PurgeActivities(f.e.Admin(), []ids.UUID{selected}, privacy.PurgeOwnerRule)
	if err != nil {
		t.Fatalf("PurgeActivities: %v", err)
	}
	if destroyed != 0 || f.erased(t, selected) {
		t.Fatalf("destroyed %d; the purge erased mail the zq floor now protects", destroyed)
	}
}

// Boot refuses a stored jurisdiction this build carries no pack for, rather
// than run selecting a law that applies nothing.
func TestBootRefusesAStoredJurisdictionWithNoPack(t *testing.T) {
	e := Setup(t)
	if err := compose.RequireCompiledInCountry(context.Background(), e.Pool); err != nil {
		t.Fatalf("unset jurisdiction refused: %v", err)
	}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `INSERT INTO setting (key, value) VALUES ('installation.country', '"zj"')`)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	err := compose.RequireCompiledInCountry(context.Background(), e.Pool)
	if err == nil || !strings.Contains(err.Error(), `"zj"`) || !strings.Contains(err.Error(), "zq") {
		t.Fatalf("boot with zj stored → %v, want a refusal naming zj and the compiled-in packs", err)
	}
}

// A jurisdiction change waits for a transaction that resolved the floor to
// destroy under it. The change cannot commit between that transaction's read
// and its write; once it ends, the change lands and the next read sees it.
func TestAJurisdictionChangeWaitsForADestroyInFlight(t *testing.T) {
	f := newFloorFixture(t)
	ctx := context.Background()
	destroying, err := f.e.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := destroying.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rolling back the destroying transaction: %v", err)
		}
	})
	if _, err := privacy.ResolveFloorPolicy(ctx, destroying, identity.CountryForDecision); err != nil {
		t.Fatal(err)
	}
	waiting, cancel := context.WithTimeout(f.governor(), 300*time.Millisecond)
	defer cancel()
	code := "zk"
	if _, err := compose.NewInstallationSettingsFor(f.e.Pool).UpdateInstallation(waiting, identity.InstallationPatch{Country: &code}); err == nil {
		t.Fatal("the jurisdiction changed while a destroying transaction held the floor it read")
	}
	if err := destroying.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	f.setCountry(t, "zk")
	if got := f.policy(t).Selected; got != "zk" {
		t.Fatalf("after the destroy ended, the floor reads %q, want the new zk", got)
	}
}

// The carry binds mail captured before the change even when its own date is
// later, as a provider's date header can be.
func TestTheCarryBindsMailCapturedBeforeTheChange(t *testing.T) {
	f := newFloorFixture(t)
	f.setCountry(t, "zk")
	f.exec(t, `UPDATE setting SET value = jsonb_set(value, '{before}', to_jsonb(now() - interval '200 days'))
		WHERE key = 'privacy.retention_floor_carry'`)
	late := f.mail(t, 150)
	f.exec(t, `UPDATE activity SET created_at = now() - interval '300 days' WHERE id = $1`, late)
	f.retentionPass(t)
	if f.erased(t, late) {
		t.Fatal("mail captured before the change was erased because its date header is later — the carry missed it")
	}
}

// Every jurisdiction change asks the retention grant, even one that weakens
// nothing: the change decides what may be destroyed.
func TestAJurisdictionChangeNeedsTheRetentionGrant(t *testing.T) {
	f := newFloorFixture(t)
	settingsOnly := f.e.As(f.e.AdminUser, nil, principal.Permissions{
		RowScope: principal.RowScopeAll,
		Objects:  map[string]principal.ObjectGrant{"installation_settings": {Read: true, Update: true}},
	})
	code := "zq"
	_, err := compose.NewInstallationSettingsFor(f.e.Pool).UpdateInstallation(settingsOnly, identity.InstallationPatch{Country: &code})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a change to an equal floor without retention_policy:update → %v, want permission denied", err)
	}
}

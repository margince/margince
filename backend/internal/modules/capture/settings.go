// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The workspace capture settings surface (CAP-PARAM-7, ADR-0072/A118). The
// value lives in the `setting` table under capture's own key (ADR-0090/A135);
// this file is the module-facing shape over it. RBAC, the audit-only posture
// and the idempotent-PATCH semantics are unchanged from the column form — the
// store below no longer owns any of them, because the settings mechanism
// carries them from the entry declaration in settingsentry.go.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Settings is the workspace-shared capture posture (the wire shape).
type Settings struct {
	AutoEnrich bool
	// SignatureEnrich is the workspace DEFAULT; a mailbox that set its own
	// switch keeps it. See settingsentry.go for why the two are separate.
	SignatureEnrich bool
	MailSharing     bool
	// SharedPostureAllowed is the workspace's answer to whether a seat may put
	// their mailbox in the `shared` posture at all.
	SharedPostureAllowed bool
	AutoEnrichDailyCap   int
	SiteRead             SiteReadLimits
}

// SiteReadLimits is what one deep read may fetch (settingssiteread.go).
type SiteReadLimits struct {
	MaxPages    int
	MaxMiB      int
	WallSeconds int
}

// SettingsPatch is a sparse capture-settings change: a nil field is left
// unchanged.
//
// A struct rather than a positional argument per setting: a caller that
// transposed two would compile, pass every test that does not happen to set
// both, and silently switch mail sharing when it meant to switch enrichment.
type SettingsPatch struct {
	AutoEnrich           *bool
	MailSharing          *bool
	SharedPostureAllowed *bool
	SignatureEnrich      *bool
	AutoEnrichDailyCap   *int
	SiteReadMaxPages     *int
	SiteReadMaxMiB       *int
	SiteReadWallSeconds  *int
}

// SettingsStore is the store over the workspace capture posture.
type SettingsStore struct {
	settings *settings.Store
}

// NewSettings builds the capture-settings store over the settings mechanism.
func NewSettings(s *settings.Store) *SettingsStore { return &SettingsStore{settings: s} }

// Get reads the workspace's capture settings. The read gate lives on the
// entry (`capture_settings`, read granted to every role), so there is no
// second gate to keep in step here.
func (s *SettingsStore) Get(ctx context.Context) (Settings, error) {
	var out Settings
	for _, f := range []struct {
		into  *bool
		entry *settings.Entry[bool]
	}{
		{&out.AutoEnrich, AutoEnrich},
		{&out.MailSharing, MailSharing},
		{&out.SignatureEnrich, SignatureEnrich},
		{&out.SharedPostureAllowed, SharedPostureAllowed},
	} {
		value, err := settings.Get(ctx, s.settings, f.entry)
		if err != nil {
			return Settings{}, fmt.Errorf("capture: reading settings: %w", err)
		}
		*f.into = value
	}
	dailyCap, err := s.dailyCap(ctx)
	if err != nil {
		return Settings{}, err
	}
	limits, err := s.SiteReadLimits(ctx)
	if err != nil {
		return Settings{}, err
	}
	out.AutoEnrichDailyCap, out.SiteRead = dailyCap, limits
	return out, nil
}

// dailyCap reads the automatic-read ceiling.
func (s *SettingsStore) dailyCap(ctx context.Context) (int, error) {
	dailyCap, err := settings.Get(ctx, s.settings, AutoEnrichDailyCap)
	if err != nil {
		return 0, fmt.Errorf("capture: reading the daily read cap: %w", err)
	}
	return dailyCap, nil
}

// SiteReadLimits reads the per-read limits, which the deep-read worker takes
// at the start of every read so a change applies to the next one. One
// transaction, so a save that changes several limits is never read half
// applied.
func (s *SettingsStore) SiteReadLimits(ctx context.Context) (SiteReadLimits, error) {
	var out SiteReadLimits
	err := s.settings.WriteTx(ctx, func(tx pgx.Tx) error {
		for _, f := range []struct {
			into  *int
			entry *settings.Entry[int]
		}{
			{&out.MaxPages, SiteReadMaxPages},
			{&out.MaxMiB, SiteReadMaxMiB},
			{&out.WallSeconds, SiteReadWallSeconds},
		} {
			value, err := settings.GetTx(ctx, tx, f.entry)
			if err != nil {
				return err
			}
			*f.into = value
		}
		return nil
	})
	if err != nil {
		return SiteReadLimits{}, fmt.Errorf("capture: reading the site-read limits: %w", err)
	}
	return out, nil
}

// AutoEnrichOn reads the auto-enrich switch alone, for the deep-read worker,
// which asks it of every automatic read it claims and needs nothing else.
func (s *SettingsStore) AutoEnrichOn(ctx context.Context) (bool, error) {
	on, err := settings.Get(ctx, s.settings, AutoEnrich)
	if err != nil {
		return false, fmt.Errorf("capture: reading the auto-enrich setting: %w", err)
	}
	return on, nil
}

// Update applies a sparse capture-settings patch (admin/ops). A nil field is
// left unchanged; an unchanged value writes nothing and audits nothing.
// Returns the settings after the write.
//
// The update gate is taken HERE, before the empty-patch branch, not left to
// the write that may never happen. An empty PATCH is still an attempt to
// change settings, and answering it from the read gate alone would let a
// read-only role probe the surface with a 200 where the column form gave a
// 403.
//
// The mirror case is real but not reachable: a caller holding update WITHOUT
// read gets a 403 on an empty patch, because the response comes from Get. No
// seeded role has that combination — `capture_settings` grants read to every
// role — so this stays a note rather than a branch.
func (s *SettingsStore) Update(ctx context.Context, patch SettingsPatch) (Settings, error) {
	if err := auth.Require(ctx, captureSettingsObject, principal.ActionUpdate); err != nil {
		return Settings{}, err
	}
	// One PATCH is one change: every field commits in one transaction or
	// neither does — a patch that half-applied would leave the caller's
	// screen agreeing with neither what they sent nor what stood before.
	err := s.settings.WriteTx(ctx, func(tx pgx.Tx) error {
		for _, f := range []struct {
			want  *bool
			entry *settings.Entry[bool]
		}{
			{patch.AutoEnrich, AutoEnrich},
			{patch.MailSharing, MailSharing},
			{patch.SharedPostureAllowed, SharedPostureAllowed},
			{patch.SignatureEnrich, SignatureEnrich},
		} {
			if f.want == nil {
				continue
			}
			if err := settings.SetTx(ctx, s.settings, tx, f.entry, *f.want); err != nil {
				return err
			}
		}
		for _, f := range []struct {
			want  *int
			entry *settings.Entry[int]
		}{
			{patch.AutoEnrichDailyCap, AutoEnrichDailyCap},
			{patch.SiteReadMaxPages, SiteReadMaxPages},
			{patch.SiteReadMaxMiB, SiteReadMaxMiB},
			{patch.SiteReadWallSeconds, SiteReadWallSeconds},
		} {
			if f.want == nil {
				continue
			}
			if err := settings.SetTx(ctx, s.settings, tx, f.entry, *f.want); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Settings{}, err
	}
	return s.Get(ctx)
}

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
	"github.com/margince/margince/backend/internal/platform/config"
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
	// MailSyncIntervalSeconds is how long a mailbox waits between syncs.
	MailSyncIntervalSeconds int
	// ConfidentialityPassCap is how many held threads one pass judges.
	ConfidentialityPassCap int
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
	AutoEnrich              *bool
	MailSharing             *bool
	SharedPostureAllowed    *bool
	SignatureEnrich         *bool
	AutoEnrichDailyCap      *int
	SiteReadMaxPages        *int
	SiteReadMaxMiB          *int
	SiteReadWallSeconds     *int
	MailSyncIntervalSeconds *int
	ConfidentialityPassCap  *int
}

// SettingsStore is the store over the workspace capture posture.
type SettingsStore struct {
	settings *settings.Store
	// config answers a value a boot source sets and refuses a change to it.
	config *config.Config
}

// NewSettings builds the capture-settings store over the settings mechanism,
// under the boot locks cfg holds.
func NewSettings(s *settings.Store, cfg *config.Config) *SettingsStore {
	return &SettingsStore{settings: s, config: cfg}
}

// Get reads the workspace's capture settings. The read gate lives on the
// entry (`capture_settings`, read granted to every role), so there is no
// second gate to keep in step here.
func (s *SettingsStore) Get(ctx context.Context) (Settings, error) {
	var out Settings
	for _, f := range []struct {
		into  *bool
		entry *config.Setting[bool]
	}{
		{&out.AutoEnrich, AutoEnrich},
		{&out.MailSharing, MailSharing},
		{&out.SignatureEnrich, SignatureEnrich},
		{&out.SharedPostureAllowed, SharedPostureAllowed},
	} {
		value, err := config.StoredVal(ctx, s.config, f.entry)
		if err != nil {
			return Settings{}, fmt.Errorf("capture: reading settings: %w", err)
		}
		*f.into = value
	}
	for _, f := range []struct {
		into  *int
		entry *config.Setting[int]
	}{
		{&out.AutoEnrichDailyCap, AutoEnrichDailyCap},
		{&out.MailSyncIntervalSeconds, MailSyncIntervalSeconds},
		{&out.ConfidentialityPassCap, ConfidentialityPassCap},
	} {
		value, err := config.StoredVal(ctx, s.config, f.entry)
		if err != nil {
			return Settings{}, fmt.Errorf("capture: reading settings: %w", err)
		}
		*f.into = value
	}
	limits, err := s.SiteReadLimits(ctx)
	if err != nil {
		return Settings{}, err
	}
	out.SiteRead = limits
	return out, nil
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
			entry *config.Setting[int]
		}{
			{&out.MaxPages, SiteReadMaxPages},
			{&out.MaxMiB, SiteReadMaxMiB},
			{&out.WallSeconds, SiteReadWallSeconds},
		} {
			value, err := config.StoredValTx(ctx, tx, s.config, f.entry)
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
	on, err := config.StoredVal(ctx, s.config, AutoEnrich)
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
			entry *config.Setting[bool]
		}{
			{patch.AutoEnrich, AutoEnrich},
			{patch.MailSharing, MailSharing},
			{patch.SharedPostureAllowed, SharedPostureAllowed},
			{patch.SignatureEnrich, SignatureEnrich},
		} {
			if f.want == nil {
				continue
			}
			if err := config.SaveTx(ctx, s.config, tx, f.entry, *f.want); err != nil {
				return err
			}
		}
		for _, f := range []struct {
			want  *int
			entry *config.Setting[int]
		}{
			{patch.AutoEnrichDailyCap, AutoEnrichDailyCap},
			{patch.SiteReadMaxPages, SiteReadMaxPages},
			{patch.SiteReadMaxMiB, SiteReadMaxMiB},
			{patch.SiteReadWallSeconds, SiteReadWallSeconds},
			{patch.MailSyncIntervalSeconds, MailSyncIntervalSeconds},
			{patch.ConfidentialityPassCap, ConfidentialityPassCap},
		} {
			if f.want == nil {
				continue
			}
			if err := config.SaveTx(ctx, s.config, tx, f.entry, *f.want); err != nil {
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

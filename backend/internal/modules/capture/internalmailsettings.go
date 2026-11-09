// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// InternalMailSelection contains choices only; provenance comes from the server.
type InternalMailSelection struct {
	HistorySince *time.Time `json:"history_since,omitempty"`
	ConnectionID ids.UUID   `json:"connection_id"`
	Containers   []string   `json:"containers"`
}

// GetInternalMailPolicy returns the administrator's current source authorizations.
func (s *SettingsStore) GetInternalMailPolicy(ctx context.Context) (InternalMailPolicy, error) {
	if err := auth.RequireAdmin(ctx); err != nil {
		return InternalMailPolicy{}, err
	}
	return settings.Get(ctx, s.settings, internalMailPolicy)
}

// SetInternalMailPolicy replaces the selected sources without granting provider access.
func (s *SettingsStore) SetInternalMailPolicy(ctx context.Context, enabled bool, selections []InternalMailSelection) (InternalMailPolicy, error) {
	if err := auth.RequireAdmin(ctx); err != nil {
		return InternalMailPolicy{}, err
	}
	if err := settings.RequireWrite(ctx, internalMailPolicy); err != nil {
		return InternalMailPolicy{}, err
	}
	if err := validateInternalMailSelections(selections); err != nil {
		return InternalMailPolicy{}, err
	}
	var next InternalMailPolicy
	err := s.settings.WriteTx(ctx, func(tx pgx.Tx) error {
		var err error
		next, err = s.setInternalMailPolicyTx(ctx, tx, enabled, selections)
		return err
	})
	return next, err
}

func buildInternalMailPolicy(ctx context.Context, tx pgx.Tx, before InternalMailPolicy, enabled bool, selections []InternalMailSelection) (InternalMailPolicy, error) {
	var now time.Time
	if err := tx.QueryRow(ctx, `SELECT transaction_timestamp()`).Scan(&now); err != nil {
		return InternalMailPolicy{}, err
	}
	next := InternalMailPolicy{Enabled: enabled, Since: before.Since, Sources: []InternalMailSource{}}
	if enabled && !before.Enabled {
		next.Since = &now
	}
	stamp, err := storekit.CapturedBy(ctx)
	if err != nil {
		return InternalMailPolicy{}, err
	}
	prior := make(map[ids.UUID]InternalMailSource, len(before.Sources))
	for _, source := range before.Sources {
		prior[source.ConnectionID] = source
	}
	ordered := slices.Clone(selections)
	slices.SortFunc(ordered, func(a, b InternalMailSelection) int {
		return strings.Compare(a.ConnectionID.String(), b.ConnectionID.String())
	})
	for _, selection := range ordered {
		source, err := enrolledInternalMailSource(ctx, tx, selection, prior[selection.ConnectionID], now, stamp, enabled)
		if err != nil {
			return InternalMailPolicy{}, err
		}
		next.Sources = append(next.Sources, source)
	}
	return next, nil
}

func enrolledInternalMailSource(ctx context.Context, tx pgx.Tx, selection InternalMailSelection, prior InternalMailSource, now time.Time, stamp string, enabled bool) (InternalMailSource, error) {
	containers := slices.Clone(selection.Containers)
	if containers == nil {
		containers = []string{}
	}
	slices.Sort(containers)
	source, err := readInternalMailSource(ctx, tx, selection.ConnectionID)
	if err != nil {
		if !enabled && errors.Is(err, apperrors.ErrNotFound) && prior.ConnectionID == selection.ConnectionID && slices.Equal(prior.Containers, containers) && sameOptionalInstant(prior.HistorySince, selection.HistorySince) {
			return prior, nil
		}
		return InternalMailSource{}, err
	}
	source.Containers, source.HistorySince = containers, selection.HistorySince
	if sameInternalMailScope(source, prior) {
		return prior, nil
	}
	if err := validateInternalMailContainers(source.Provider, containers); err != nil {
		return InternalMailSource{}, err
	}
	source.Containers, source.EnrolledAt, source.EnrolledBy = containers, now, stamp
	return source, nil
}

func validateInternalMailSelections(selections []InternalMailSelection) error {
	if len(selections) > 200 {
		return apperrors.ErrInvalidArgument
	}
	seen := make(map[ids.UUID]bool, len(selections))
	for _, source := range selections {
		if source.ConnectionID == ids.Nil || seen[source.ConnectionID] || len(source.Containers) > 200 {
			return apperrors.ErrInvalidArgument
		}
		seen[source.ConnectionID] = true
	}
	return nil
}

func (s *SettingsStore) setInternalMailPolicyTx(ctx context.Context, tx pgx.Tx, enabled bool, selections []InternalMailSelection) (InternalMailPolicy, error) {
	if err := settings.RequireWrite(ctx, internalMailPolicy); err != nil {
		return InternalMailPolicy{}, err
	}
	if err := validateInternalMailSelections(selections); err != nil {
		return InternalMailPolicy{}, err
	}
	if err := settings.LockForWrite(ctx, tx, internalMailPolicyKey); err != nil {
		return InternalMailPolicy{}, err
	}
	before, err := settings.ApplyTx(ctx, tx, internalMailPolicy)
	if err != nil {
		return InternalMailPolicy{}, err
	}
	next, err := buildInternalMailPolicy(ctx, tx, before, enabled, selections)
	if err != nil {
		return InternalMailPolicy{}, err
	}
	return next, settings.SetTx(ctx, s.settings, tx, internalMailPolicy, next)
}

// InternalMailSettings is the administrator's policy together with eligible connections.
type InternalMailSettings struct {
	Policy           InternalMailPolicy
	AvailableSources []InternalMailSourceOption
}

// InternalMailPatch is an explicit replacement of the authorized source set.
type InternalMailPatch struct {
	Enabled bool
	Sources []InternalMailSelection
}

//nolint:nilnil // Other roles receive capture settings without this optional administrator section.
func (s *SettingsStore) internalMailSettings(ctx context.Context) (*InternalMailSettings, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || !slices.Contains(actor.Permissions.RoleKeys, "admin") {
		return nil, nil
	}
	policy, err := s.GetInternalMailPolicy(ctx)
	if err != nil {
		return nil, err
	}
	sources, err := s.ListInternalMailSources(ctx)
	if err != nil {
		return nil, err
	}
	return &InternalMailSettings{Policy: policy, AvailableSources: sources}, nil
}

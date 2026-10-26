// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The settings and writers a proof's life needs from other modules. identity
// owns the installation's zone and privacy owns the tombstone of a rights
// case; consent imports neither, so compose injects both.

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// InstallationZoneFunc reads the installation's time zone on the caller's
// transaction.
type InstallationZoneFunc func(ctx context.Context, tx pgx.Tx) (*time.Location, error)

// RightsCaseRetirer takes the requester off one closed case, skipping one under
// dispute, and answers whether it did.
type RightsCaseRetirer func(ctx context.Context, tx pgx.Tx, caseID ids.UUID) (bool, error)

// WithInstallationZone injects the zone a proof's retention date is counted in.
func (s *Store) WithInstallationZone(zone InstallationZoneFunc) *Store {
	s.zone = zone
	return s
}

// WithInstallationZone is the handlers-side spelling.
func (h Handlers) WithInstallationZone(zone InstallationZoneFunc) Handlers {
	h.store = h.store.WithInstallationZone(zone)
	return h
}

// WithRightsCaseRetirer injects the writer an expired proof ends its case with.
func (s *Store) WithRightsCaseRetirer(retire RightsCaseRetirer) *Store {
	s.retireCase = retire
	return s
}

// installationZone answers the zone, or UTC, the setting's own default, on a
// store nothing wired.
func (s *Store) installationZone(ctx context.Context, tx pgx.Tx) (*time.Location, error) {
	if s.zone == nil {
		return time.UTC, nil
	}
	return s.zone(ctx, tx)
}

// errNoRetirer refuses an expiry pass that could delete a proof but not end
// the reference the proof outlived.
var errNoRetirer = errors.New("consent: the proof expiry has no rights-case retirer wired")

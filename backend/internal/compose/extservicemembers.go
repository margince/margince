// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The service member each landing unit runs its feed as.
//
// A unit that declares an ingress source lands records on a schedule, and that
// needs a member nobody signs in as. Installing the unit is what says so, so the
// boot makes the member and an admin does not have to declare it by hand.

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/pkg/extension"
)

// EnsureExtensionServiceMembers makes the service member of every composed unit
// that declares an ingress source and has none yet. Every process role calls it
// once the installation is bound. The make is idempotent and serialized, so the
// api and the worker booting together make one member each.
//
// Before bootstrap there is no workspace, and before the installation has
// described its own company the seat gate refuses any seat. Both are "not yet":
// the next boot after onboarding makes the member.
func EnsureExtensionServiceMembers(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger, exts []extension.Extension) error {
	var want []identity.ServiceMemberInput
	for _, e := range exts {
		if len(e.Ingress) > 0 {
			want = append(want, identity.ServiceMemberInput{Extension: string(e.Name), DisplayName: string(e.Name) + " feed"})
		}
	}
	if len(want) == 0 {
		return nil
	}
	ctx, bootstrapped, err := bootLedgerScope(ctx, pool, "system:service-members")
	if err != nil || !bootstrapped {
		return err
	}
	described, err := installationDescribed(contacts.NewStore(InstallationDB(pool)))(ctx)
	if err != nil {
		return err
	}
	if !described {
		log.Info("extension service members not made: the installation has not described its company yet")
		return nil
	}
	made, err := identity.NewService(pool).EnsureServiceMembers(ctx, want)
	if err != nil {
		return err
	}
	// Logged after the commit, so it never reports a make that rolled back.
	if made > 0 {
		log.Info("extension service members made", "members", made)
	}
	return nil
}

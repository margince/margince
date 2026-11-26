// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The two facts an ingest establishes about the member it runs as, and the
// declaration lookup that decides whether the unit may ingest at all.
//
// They live beside the port rather than inside it because each is a question
// with one answer the whole tier shares: has this member asked THIS unit to act
// for them, and what may they do right now. Neither is the unit's to assert.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
	"github.com/margince/margince/backend/pkg/extension"
)

// composedFor returns what the named unit declared in THIS boot's composition,
// read through field.
//
// It reads the composed declarations rather than a second registry, for the
// reason ComposedExtensions exists: the set the boot reconciliation actually
// validated is the only honest answer to "what may this unit do", and a
// parallel copy could describe a unit that is not serving.
func composedFor[T any](unit string, field func(extension.Extension) []T) []T {
	for _, ext := range ComposedExtensions() {
		if string(ext.Name) == unit {
			return field(ext)
		}
	}
	return nil
}

func composedIngressFor(unit string) []extension.IngressSource {
	return composedFor(unit, func(e extension.Extension) []extension.IngressSource { return e.Ingress })
}

func composedChannelsFor(unit string) []extension.Channel {
	return composedFor(unit, func(e extension.Extension) []extension.Channel { return e.Channels })
}

// declaredUserSecretKeys are the user-scoped secret keys the named unit
// declares in THIS boot's composition.
//
// The consent check reads them rather than accepting any row in the unit's
// namespace, and the difference is a real one: extension_secret keeps a
// mapping row after a unit stops declaring the key it was deposited under, so
// a credential a member gave an earlier version of a unit would go on
// authorizing a capability the current manifest does not describe. What an
// operator can read has to be what the core acts on.
func declaredUserSecretKeys(unit string) []string {
	var keys []string
	for _, ext := range ComposedExtensions() {
		if string(ext.Name) != unit {
			continue
		}
		for _, request := range ext.Secrets {
			if request.Scope == extension.SecretScopeUser {
				keys = append(keys, request.Key)
			}
		}
	}
	return keys
}

// extensionMemberConsented reports whether the member currently holds a
// user-scoped secret under one of this unit's DECLARED keys.
//
// It also subsumes the refusal of the installation's own agent-runner seat,
// which is why no separate check for it exists: that seat has no password and
// no session, so it can never have deposited a credential through the screen
// that deposits them. A check that can only ever agree with this one would be
// two spellings of a single fact, which is how the two drift apart later.
//
// DEPOSITING A CREDENTIAL IS THE CONSENT ACT. A member who pastes their
// provider token into a unit's screen is saying "poll this account for me", and
// that is the fact an ingest needs before it may act as them — without it a unit
// could name any colleague and land records on their authority, which is the
// confused deputy this check exists to close.
//
// It reads the MAPPING ROW only, never the material: the question is whether a
// credential is on deposit, and unsealing one to answer it would spend the
// custodian and hand this path a secret it has no use for.
//
// The read is the installation's: ADR-0091 §8 phase D took the tenant column
// off extension_secret and then off app_user, and one installation serves one
// company (ADR-0061), so the deposit either exists or it does not.
func extensionMemberConsented(ctx context.Context, pool *pgxpool.Pool, unit string, member ids.UUID) (bool, error) {
	if pool == nil {
		return false, errExtensionRuntimeUnwired
	}
	if len(declaredUserSecretKeys(unit)) == 0 {
		return false, nil
	}
	var consented bool
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		consented, err = memberConsentedTx(ctx, tx, unit, member)
		return err
	})
	if err != nil {
		return false, fmt.Errorf("compose: reading whether a member has deposited a credential with %s: %w", unit, err)
	}
	return consented, nil
}

// memberConsentedTx is extensionMemberConsented on a transaction already held,
// for a door that may not take a second connection. A service member consents
// for the unit it was declared for and no other. Without the join, a unit could
// name another unit's service member and borrow the authority the operator gave
// that feed.
func memberConsentedTx(ctx context.Context, tx pgx.Tx, unit string, member ids.UUID) (bool, error) {
	declared := declaredUserSecretKeys(unit)
	if len(declared) == 0 {
		// Such a unit gives a member no way to consent. Nobody is left to act for.
		return false, nil
	}
	var consented bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM extension_secret s
			JOIN app_user u ON u.id = s.user_id
			WHERE s.extension_name = $1
			  AND s.user_id = $2
			  AND s.key = ANY($3)
			  AND (u.service_extension IS NULL OR u.service_extension = $1)
		)`, unit, member, declared).Scan(&consented)
	return consented, err
}

// memberConnectedTx reports whether the member is active now and holds one of
// the unit's user-scoped secrets. A member who has left keeps the credential
// they deposited. A door that acts for a named member asks this, not the deposit.
func memberConnectedTx(ctx context.Context, tx pgx.Tx, unit string, member ids.UUID) (bool, error) {
	var live bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM app_user u WHERE u.id = $1 AND `+workforce.LiveMemberSQL("u")+`)`, member).Scan(&live); err != nil || !live {
		return false, err
	}
	return memberConsentedTx(ctx, tx, unit, member)
}

// liveMemberPrincipal resolves what the member may do now, per call, so a
// demotion binds from the next record. An archived, suspended or gone member
// answers ErrNotFound and lands nothing. The pool-built resolver is why the
// runtime binding needs no entry for it.
func liveMemberPrincipal(ctx context.Context, pool *pgxpool.Pool, workspace, member ids.UUID) (principal.Principal, error) {
	if pool == nil {
		return principal.Principal{}, errExtensionRuntimeUnwired
	}
	return authz.MemberPrincipal(ctx, identity.NewService(pool), workspace, member)
}

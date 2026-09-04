// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// One installation serves one organization (A107/ADR-0061). The workspace
// row remains the internal singleton boundary: this file owns its
// boot-time creation from deployment configuration and its resolution for
// every request. The invariant is enforced here, at boot and at lookup —
// deliberately NOT as a schema constraint, so cross-tenant RLS tests keep
// proving isolation by inserting a second workspace directly.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity/internal/password"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// ErrNotBootstrapped means the database holds no active organization.
// The API refuses to serve (it bootstraps at boot or dies); the worker
// retries until the API has bootstrapped; the MCP binary exits with this
// as an operator error — pre-bootstrap no human exists who could have
// granted a passport.
var ErrNotBootstrapped = errors.New("identity: installation not bootstrapped — no active organization exists")

// ErrMultipleWorkspaces means the database violates the
// single-organization invariant. Never auto-resolved — an operator
// explicitly retains one organization and archives the rest (ADR-0061 §3).
var ErrMultipleWorkspaces = errors.New("identity: more than one active workspace — the single-organization invariant requires an operator-led migration")

// installationLockKey serializes bootstrap across concurrently starting
// processes (pg_advisory_xact_lock). The value is arbitrary but fixed —
// every binary of this installation must agree on it.
const installationLockKey = int64(0x4d61726761_0001) // "Marga"+1

// InstallationBootstrap is the creation input for the singleton organization.
//
// It arrives one of two ways and never both: from the deployment
// configuration file when the operator configured a bootstrap admin, or from
// the setup claim when a human claims an unprovisioned installation with the
// setup token. The two paths are mutually exclusive — creating the
// organization retires every setup token — so neither can correct the other
// afterwards.
type InstallationBootstrap struct {
	OrganizationName string
	BaseCurrency     string
	BaseLanguage     string
	Timezone         string
	AdminEmail       string
	AdminName        string
	AdminPassword    string
}

// BootstrapInstallation binds the installation to its singleton
// organization, creating it when the database is empty. Under a
// transaction-scoped advisory lock (so concurrent API starts cannot race
// a second organization into existence) it applies the ADR-0061 state
// machine: 0 active workspaces → create organization + first admin +
// system roles + seeds atomically; 1 → bind to it; >1 → refuse.
//
// create is nil when no bootstrap_admin is configured — then an empty
// database is ErrNotBootstrapped instead of being claimable. No session
// is minted: the first admin signs in through the normal login, and
// bootstrap values never reconcile into an existing organization
// (restart never resets a password, role, or seed).
//
// create is a FUNCTION, and it is called only on the branch that creates
// the organization. Resolving the bootstrap input eagerly would read the
// admin's password secret on every boot of an already-bootstrapped
// installation — a secret ADR-0061 §2 says may be deleted once the
// organization exists, so the read would fail on exactly the installations
// that followed the ADR. What is only needed to CREATE is only resolved
// when creating.
// discarded names the identity settings the caller supplied that a previous
// installation's rows already occupied — empty on a first bootstrap, and empty
// on the bind branch, which supplies nothing. It is returned rather than logged
// because what a re-bootstrap SHOULD do with them is an open product question
// (#863); refusing to be silent is the part that needs no ruling.
func (s *Service) BootstrapInstallation(ctx context.Context, create func() (InstallationBootstrap, error), seed func(ctx context.Context, tx pgx.Tx) error) (wsID ids.WorkspaceID, created bool, discarded []string, err error) {
	err = database.WithInfraTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, installationLockKey); err != nil {
			return fmt.Errorf("identity: taking the bootstrap advisory lock: %w", err)
		}
		existing, err := activeWorkspaces(ctx, tx)
		if err != nil {
			return err
		}
		switch {
		case len(existing) == 1:
			wsID = existing[0]
			return nil
		case len(existing) > 1:
			return ErrMultipleWorkspaces
		case create == nil:
			return ErrNotBootstrapped
		}
		in, err := create()
		if err != nil {
			return err
		}
		wsID, err = createInstallation(ctx, tx, in, originConfigured, seed, &discarded)
		created = err == nil
		return err
	})
	if err != nil {
		return ids.WorkspaceID{}, false, nil, err
	}
	s.installation.Store(&wsID)
	return wsID, created, discarded, nil
}

// InstallationWorkspace resolves the singleton organization for a
// request, cached after the first successful lookup — the resolution the
// per-request slug used to provide. Pre-bootstrap lookups return
// ErrNotBootstrapped (never cached: the worker polls this until the API
// bootstraps, then binds).
func (s *Service) InstallationWorkspace(ctx context.Context) (ids.WorkspaceID, error) {
	if cached := s.installation.Load(); cached != nil {
		return *cached, nil
	}
	var wsID ids.WorkspaceID
	err := database.WithInfraTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		existing, err := activeWorkspaces(ctx, tx)
		if err != nil {
			return err
		}
		switch len(existing) {
		case 0:
			return ErrNotBootstrapped
		case 1:
			wsID = existing[0]
			return nil
		default:
			return ErrMultipleWorkspaces
		}
	})
	if err != nil {
		return ids.WorkspaceID{}, err
	}
	s.installation.Store(&wsID)
	return wsID, nil
}

// activeWorkspaces lists un-archived workspace ids. LIMIT 3: the caller
// only distinguishes zero, one, and too-many.
func activeWorkspaces(ctx context.Context, tx pgx.Tx) ([]ids.WorkspaceID, error) {
	rows, err := tx.Query(ctx, `SELECT id FROM workspace WHERE archived_at IS NULL LIMIT 3`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ids.WorkspaceID
	for rows.Next() {
		var id ids.WorkspaceID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// provisioningOrigin says which of ADR-0105's two paths created the
// organization, and it decides two things that follow from the same fact —
// whether a human was present. Who the creation is attributed to, and whether
// the first admin owes a password rotation, are both answers to that. It is a
// parameter rather than a field on InstallationBootstrap because it describes
// how the input arrived, not what the input is.
type provisioningOrigin int

const (
	// originConfigured — margince.yaml carried bootstrap_admin and boot applied
	// it. No human was present, so the record says `system`.
	originConfigured provisioningOrigin = iota
	// originClaimed — a human presented the operator's setup token and chose
	// their own credential. The record names the admin that request created:
	// the first provisioning event in this product with someone behind it.
	originClaimed
)

// createInstallation writes organization + first admin + system roles +
// module seeds in the caller's transaction — either everything exists
// afterwards or nothing does (the ADR-0043 bootstrap atomicity, kept).
// discarded is an out-parameter rather than a third return value, and that is a
// deliberate trade. This function has ten error returns; making the discards a
// return value would restate every one of them to add a `nil`, which turns ten
// untouched error paths into lines that read as changed to anything comparing
// against the previous revision. The values it collects are a second OUTCOME of
// a successful create, not a second result of the call.
func createInstallation(ctx context.Context, tx pgx.Tx, in InstallationBootstrap, origin provisioningOrigin, seed func(ctx context.Context, tx pgx.Tx) error, discarded *[]string) (ids.WorkspaceID, error) {
	boot := BootstrapInput{
		WorkspaceName: in.OrganizationName,
		AdminEmail:    in.AdminEmail,
		AdminName:     in.AdminName,
		AdminPassword: in.AdminPassword,
		Timezone:      in.Timezone,
	}
	if err := boot.normalize(); err != nil {
		return ids.WorkspaceID{}, err
	}
	currency, language, err := resolveBasis(in, origin)
	if err != nil {
		return ids.WorkspaceID{}, err
	}
	hash, err := password.Hash(boot.AdminPassword)
	if err != nil {
		return ids.WorkspaceID{}, err
	}

	var wsID ids.WorkspaceID
	// The row is identity and lifecycle now, nothing else: ADR-0090 moved the
	// installation's configuration into `setting`, and ADR-0091 retired the
	// slug it used to carry. The derived slug went with the seeded agent seat,
	// whose local address was its last reader.
	if err := tx.QueryRow(ctx,
		`INSERT INTO workspace DEFAULT VALUES RETURNING id`).Scan(&wsID); err != nil {
		return ids.WorkspaceID{}, err
	}
	// The installation's identity is written HERE, not read back off the row
	// by a caller: normalize() and the currency default above are the only
	// place these values are resolved, and a second derivation elsewhere would
	// drift from them (the columns that used to carry them are gone).
	dropped, err := seedInstallationIdentity(ctx, tx, installationIdentity{
		name:     boot.WorkspaceName,
		zone:     boot.Timezone,
		currency: currency,
		language: language,
	})
	if err != nil {
		return ids.WorkspaceID{}, err
	}
	*discarded = dropped

	var userID ids.UserID
	if err := tx.QueryRow(ctx,
		`INSERT INTO app_user (email, password_hash, display_name, timezone, must_change_password)
		 VALUES (lower($1), $2, $3, $4, $5) RETURNING id`,
		boot.AdminEmail, hash, boot.AdminName, boot.Timezone,
		origin == originConfigured).Scan(&userID); err != nil {
		return ids.WorkspaceID{}, err
	}
	if err := seedSystemRoles(ctx, tx, userID); err != nil {
		return ids.WorkspaceID{}, err
	}
	// Any outstanding claim credential is retired here, on BOTH paths. The
	// claim path has already spent it; the configured path never issued one but
	// may be running on an installation that was minted a token by an earlier
	// unprovisioned boot. Left alive, that token would sit in a log pipeline
	// while /setup/status advertised the installation as claimable — inert only
	// while an organization exists, and live again the moment one is archived or
	// the database is restored empty. The organization is what retires it, not
	// whichever path created the organization.
	if err := retireSetupTokens(ctx, tx); err != nil {
		return ids.WorkspaceID{}, err
	}
	// Who did this. A configured bootstrap is a SYSTEM event — no human signed
	// in, and naming one would make the record assert something false. A CLAIM
	// is the opposite: someone presented the operator's token and chose their
	// own credential in that same request, so the record names the admin it
	// created (ADR-0105 §4). The module seeds written inside this transaction
	// stay `system:seed` on both paths (B37) — a claimant chose an organization
	// name, not a default pipeline's stages.
	actorType, actorID := "system", "installation-bootstrap"
	if origin == originClaimed {
		// 'human' is the vocabulary the audit surface already speaks
		// (human|agent|connector|system); a claim is a person acting, so it
		// takes the existing word rather than widening the enum for one row.
		actorType, actorID = "human", userID.String()
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO system_log (actor_type, actor_id, action, detail)
		 VALUES ($1, $2, 'installation_bootstrap', jsonb_build_object('admin_user_id', $3::text))`,
		actorType, actorID, userID.String()); err != nil {
		return ids.WorkspaceID{}, err
	}

	if seed != nil {
		// Boot bootstrap IS the originating operation: it mints the one
		// correlation id its seed writes (pipeline.created, …) trace to —
		// the id the HTTP middleware would have minted per request.
		seedCtx := principal.WithActor(principal.WithWorkspaceID(ctx, wsID.UUID), principal.Principal{
			Type: principal.PrincipalSystem, ID: "system",
		})
		seedCtx = principal.WithCorrelationID(seedCtx, ids.NewV7())
		if err := seed(seedCtx, tx); err != nil {
			return ids.WorkspaceID{}, err
		}
	}
	return wsID, nil
}

// resolveBasis settles what the installation is measured in — the currency
// every amount converts to, and the language AI writes the shared record in.
//
// The two provisioning paths get different answers to an ABSENT value, and the
// difference is whether a human was there to ask. A deployment file has no one
// at boot, so an omitted key falls back and the installation starts; the
// operator corrects it in Settings afterwards. A claim has a human in front of
// a form that asks, so an absent value means a client that stopped asking, and
// defaulting it would put the installation permanently on EUR because nobody
// was ever shown the question — the base currency stops being changeable once
// anything converts against it.
//
// It runs INSIDE the claim transaction, after the setup token has matched, so
// a caller holding no valid token learns nothing about this body's shape.
// The validators are the ENTRIES' own, so what the claim accepts and what the
// settings screen accepts are one rule with one spelling. A refusal carries the
// setting key as its field, which is what a 422 needs to point at.
func resolveBasis(in InstallationBootstrap, origin provisioningOrigin) (currency, language string, err error) {
	currency, language = in.BaseCurrency, in.BaseLanguage
	if origin == originClaimed {
		for _, ask := range []struct {
			entry *settings.Entry[string]
			value string
		}{
			{BaseCurrency, currency},
			{BaseLanguage, language},
		} {
			raw, err := json.Marshal(ask.value)
			if err != nil {
				return "", "", fmt.Errorf("identity: encoding %s from the claim: %w", ask.entry.Key(), err)
			}
			if err := ask.entry.ValidateJSON(raw); err != nil {
				return "", "", err
			}
		}
		return currency, language, nil
	}
	if currency == "" {
		currency = "EUR"
	}
	if language == "" {
		language = string(textlang.English)
	}
	return currency, language, nil
}

// installationIdentity is what the installation IS: the four values seeded
// together at bootstrap. Named fields rather than four positional strings,
// because three of them are same-typed and a transposed pair would seed a
// currency into the timezone row without a compiler complaint.
type installationIdentity struct {
	name     string
	zone     string
	currency string
	language string
}

// seedInstallationIdentity writes the settings rows that ARE the
// installation's identity (ADR-0090/A135). Seed, not Set: this runs inside
// bootstrap's own transaction, before any principal exists to gate a settings
// write, and it is creating the values rather than changing them.
//
// It answers the keys that were NOT stored. `setting` is not tenant-scoped, so a
// bootstrap over a database that already holds an installation's rows creates a
// new workspace beside the old settings and every value the operator supplied is
// discarded — which is #863. What should happen then is undecided; this only
// makes sure the caller can say it happened.
func seedInstallationIdentity(ctx context.Context, tx pgx.Tx, id installationIdentity) (discarded []string, err error) {
	for _, seed := range []struct {
		entry *settings.Entry[string]
		value string
	}{
		{Name, id.name},
		{Timezone, id.zone},
		{BaseCurrency, id.currency},
		{BaseLanguage, id.language},
	} {
		stored, err := settings.SeedValue(ctx, tx, seed.entry, seed.value)
		if err != nil {
			return nil, err
		}
		if !stored {
			discarded = append(discarded, seed.entry.Key())
		}
	}
	return discarded, nil
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// A seat's OTHER addresses: a send-as alias, a private domain the same person
// reads, an address they forward from.
//
// Mail among a person's own addresses is not correspondence with anybody, and
// an alias is not a contact. The sink reads this list twice — once to decide a
// message is wholly internal (internalOnlyTx) and once to decide who the
// creation ladder is even about (ladderSubjectTx) — so an address a seat
// declares as their own can never become a person record.
//
// Per USER, never per workspace. One seat's alias says nothing about another
// seat's mail, and a workspace-wide list would let anyone silence a colleague's
// counterparty by claiming their address. The workspace's own mail DOMAINS are
// a different list (capture_own_domain) saying "we are all colleagues here";
// this one says "that is also me".

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The identity vocabulary, the Go spelling of the table's CHECKs. The kinds
// are the exclusion kinds by design: both lists answer "which addresses does
// this rule cover", and one spelling of address-or-domain is what lets them
// share ValidExclusionValue rather than growing a second parser that folds
// case differently on some path nobody tests.
const (
	IdentityKindAddress = ExclusionKindAddress
	IdentityKindDomain  = ExclusionKindDomain

	// IdentitySourceUser is a seat typing in their own address.
	IdentitySourceUser = "user"
	// IdentitySourceProvider is an address a provider attests. Nothing writes
	// it yet: reading Gmail's sendAs list needs a scope the grant does not
	// request, and asking for a wider scope is a decision about what the
	// product asks of a user rather than a detail of this store.
	IdentitySourceProvider = "provider"
)

// OwnerIdentity is one of a seat's own addresses as stored.
type OwnerIdentity struct {
	ID        ids.UUID
	UserID    ids.UUID
	Kind      string
	Value     string
	Source    string
	CreatedAt time.Time
}

// OwnerIdentityStore reads and writes the per-seat identity list.
type OwnerIdentityStore struct {
	db *database.DB
}

// NewOwnerIdentityStore builds the store over the app pool.
func NewOwnerIdentityStore(db *database.DB) *OwnerIdentityStore {
	return &OwnerIdentityStore{db: db}
}

// List answers the caller's own identities and nobody else's. A colleague's
// alias is not a workspace fact: it says which of their mail is private, which
// is the thing this list exists to protect.
func (s *OwnerIdentityStore) List(ctx context.Context) ([]OwnerIdentity, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return nil, err
	}
	actor, err := storekit.Actor(ctx)
	if err != nil {
		return nil, err
	}
	var out []OwnerIdentity
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id, user_id, kind, value, source, created_at
			  FROM capture_owner_identity
			 WHERE user_id = $1
			 ORDER BY kind, value`, actor.UserID)
		if err != nil {
			return fmt.Errorf("capture: listing owner identities: %w", err)
		}
		defer rows.Close()
		out, err = pgx.CollectRows(rows, scanOwnerIdentity)
		if err != nil {
			return fmt.Errorf("capture: listing owner identities: %w", err)
		}
		return nil
	})
	return out, err
}

// scanOwnerIdentity reads one row in the column order this table's SELECT list
// uses. It is a function rather than an inline loop body so a second reader
// takes the same order rather than restating it.
func scanOwnerIdentity(row pgx.CollectableRow) (OwnerIdentity, error) {
	var identity OwnerIdentity
	err := row.Scan(&identity.ID, &identity.UserID, &identity.Kind,
		&identity.Value, &identity.Source, &identity.CreatedAt)
	return identity, err
}

// Add records one of the caller's own addresses. It takes a human seat and
// nothing more: a person claiming their own address needs no grant, and the
// claim binds only their own mail. Idempotent on the folded value, so re-adding
// answers the existing row rather than refusing.
func (s *OwnerIdentityStore) Add(ctx context.Context, kind, raw string) (OwnerIdentity, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return OwnerIdentity{}, err
	}
	actor, err := storekit.Actor(ctx)
	if err != nil {
		return OwnerIdentity{}, err
	}
	value, err := ValidExclusionValue(kind, raw)
	if err != nil {
		return OwnerIdentity{}, err
	}
	var out OwnerIdentity
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := tx.QueryRow(ctx, `
			INSERT INTO capture_owner_identity (user_id, kind, value, source, created_by)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (user_id, kind, value) DO UPDATE SET value = EXCLUDED.value
			RETURNING id, user_id, kind, value, source, created_at`,
			actor.UserID, kind, value, IdentitySourceUser, actor.ID).
			Scan(&out.ID, &out.UserID, &out.Kind, &out.Value, &out.Source, &out.CreatedAt); err != nil {
			return fmt.Errorf("capture: adding an owner identity: %w", err)
		}
		// Audit-only, like the exclusion list and the own-domain list beside
		// it: this is capture configuration, and the closed event catalog
		// (events.md) carries no type for it.
		_, err := storekit.AuditEvent(ctx, tx, "update", captureSettingsObject,
			storekit.MustWorkspace(ctx), ownerIdentityAuditImage(out))
		return err
	})
	return out, err
}

// Remove withdraws one claim. Somebody else's is not theirs to see, so it
// answers not-found like a row that is not there — the same shape the
// exclusion list uses, and for the same reason: a refusal that distinguished
// "not yours" from "not there" would confirm the colleague's alias exists.
func (s *OwnerIdentityStore) Remove(ctx context.Context, id ids.UUID) error {
	if err := auth.RequireHuman(ctx); err != nil {
		return err
	}
	actor, err := storekit.Actor(ctx)
	if err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		var identity OwnerIdentity
		err := tx.QueryRow(ctx, `
			SELECT id, user_id, kind, value, source FROM capture_owner_identity WHERE id = $1`, id).
			Scan(&identity.ID, &identity.UserID, &identity.Kind, &identity.Value, &identity.Source)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("capture: reading an owner identity: %w", err)
		}
		if identity.UserID != actor.UserID {
			return apperrors.ErrNotFound
		}
		if _, err := tx.Exec(ctx, `DELETE FROM capture_owner_identity WHERE id = $1`, id); err != nil {
			return fmt.Errorf("capture: removing an owner identity: %w", err)
		}
		// "archive", not a delete verb: the audit vocabulary is closed and
		// withdrawing a claim IS retiring it, as the exclusion list reads it.
		_, err = storekit.Audit(ctx, tx, "archive", captureSettingsObject,
			storekit.MustWorkspace(ctx), ownerIdentityAuditImage(identity), nil)
		return err
	})
}

// ownerIdentityAuditImage is what the trail records about a claim: its id and
// kind, and never its VALUE.
//
// This is the same ruling the user-scoped exclusion follows, and for a stronger
// reason: an owner identity is always one person's, and its whole purpose is to
// keep a private address out of the CRM. Writing it into the audit log would
// put it back in, where nothing erases it and every admin reads it. The
// id and kind answer "who declared an address, and when", which is what an
// auditor asks; which address is the seat's own business.
func ownerIdentityAuditImage(identity OwnerIdentity) map[string]any {
	return map[string]any{"id": identity.ID, auditKeyKind: identity.Kind}
}

// ownerIdentitiesTx is the acting seat's identities as a SelfSet the sink's
// gates test addresses against, or an empty set when no seat is acting (a
// background sweep, a system principal). An empty set changes no decision: it
// leaves every gate exactly where it was before a seat declared anything.
func ownerIdentitiesTx(ctx context.Context, tx pgx.Tx) (SelfSet, error) {
	user := actorUserID(ctx)
	if user == ids.Nil {
		return SelfSet{}, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT kind, value FROM capture_owner_identity WHERE user_id = $1`, user)
	if err != nil {
		return SelfSet{}, fmt.Errorf("capture: reading the mailbox owner's identities: %w", err)
	}
	defer rows.Close()
	var addresses, domains []string
	for rows.Next() {
		var kind, value string
		if err := rows.Scan(&kind, &value); err != nil {
			return SelfSet{}, fmt.Errorf("capture: reading the mailbox owner's identities: %w", err)
		}
		if kind == IdentityKindDomain {
			domains = append(domains, value)
			continue
		}
		addresses = append(addresses, value)
	}
	if err := rows.Err(); err != nil {
		return SelfSet{}, fmt.Errorf("capture: reading the mailbox owner's identities: %w", err)
	}
	return NewSelfSet(addresses, domains), nil
}

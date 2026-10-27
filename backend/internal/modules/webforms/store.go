// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webforms

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// defaultLeadOrigin is the lead_origin a form names when its admin chose none.
const defaultLeadOrigin = "website"

// Store owns web_form. vault is nil on an installation with no secret store:
// browser forms still work, and every signed sender is refused at create.
type Store struct {
	db    *database.DB
	vault keyvault.Vault
	log   *slog.Logger
}

// NewStore wires the forms table over the workspace database.
func NewStore(db *database.DB, log *slog.Logger) *Store {
	return &Store{db: db, log: log}
}

// WithVault binds the secret store signed senders keep their secret in.
func (s *Store) WithVault(vault keyvault.Vault) *Store {
	clone := *s
	clone.vault = vault
	return &clone
}

// SecretsAvailable reports whether a signed sender can be configured here.
func (s *Store) SecretsAvailable() bool { return s.vault != nil }

// CreateInput is a new form as an admin describes it. The key, the captured
// by and a server form's secret are minted here, never sent.
type CreateInput struct {
	Name                 string
	Sender               string
	AllowedOrigins       []string
	SenderSecret         *string
	LeadOrigin           *string
	OwnerID              *ids.UUID
	FieldMap             map[string]string
	ConsentRequired      bool
	ConsentPolicyVersion *string
	ConsentWording       *string
	RedirectURL          *string
	SuccessMessage       *string
}

func (in *CreateInput) normalize() error {
	name, err := checkName(in.Name)
	if err != nil {
		return err
	}
	in.Name = name
	if err := checkSender(in.Sender); err != nil {
		return err
	}
	if in.AllowedOrigins, err = normalizeOrigins(in.AllowedOrigins); err != nil {
		return err
	}
	switch {
	case in.Sender == SenderBrowser && len(in.AllowedOrigins) == 0:
		return badInput("allowed_origins", "a browser form needs the website it is posted from")
	case in.Sender != SenderBrowser && len(in.AllowedOrigins) > 0:
		return badInput("allowed_origins", "only a browser form takes posts by website; a %s form is admitted by its signature", in.Sender)
	case builderSender(in.Sender) && (in.SenderSecret == nil || *in.SenderSecret == ""):
		return badInput("sender_secret", "a %s form needs the secret %s signs its webhook with", in.Sender, in.Sender)
	case !builderSender(in.Sender) && in.SenderSecret != nil:
		return badInput("sender_secret", "only a builder form takes a pasted secret")
	}
	if in.LeadOrigin == nil {
		origin := defaultLeadOrigin
		in.LeadOrigin = &origin
	}
	return checkSettings(in.FieldMap, in.ConsentRequired, in.ConsentWording, in.RedirectURL, in.SuccessMessage, in.SenderSecret)
}

func checkSettings(fieldMap map[string]string, consentRequired bool, wording, redirect, success, secret *string) error {
	if err := checkFieldMap(fieldMap); err != nil {
		return err
	}
	if err := checkConsent(consentRequired, wording); err != nil {
		return err
	}
	if err := checkRedirect(redirect); err != nil {
		return err
	}
	if err := checkBounded("success_message", success, maxSuccessMessage); err != nil {
		return err
	}
	return checkSenderSecret(secret)
}

// CreateForm registers a form. For a server form it answers the signing
// secret it minted; that is the only time the secret leaves the vault.
func (s *Store) CreateForm(ctx context.Context, in CreateInput) (Form, string, error) {
	if err := auth.Require(ctx, rbacObject, principal.ActionCreate); err != nil {
		return Form{}, "", err
	}
	if err := in.normalize(); err != nil {
		return Form{}, "", err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return Form{}, "", err
	}
	key, err := mintFormKey()
	if err != nil {
		return Form{}, "", err
	}
	ref, minted, err := s.storeSecret(ctx, in.Sender, in.SenderSecret)
	if err != nil {
		return Form{}, "", err
	}
	var out Form
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := checkReferences(ctx, tx, in.LeadOrigin, in.OwnerID); err != nil {
			return err
		}
		if in.FieldMap == nil {
			in.FieldMap = map[string]string{}
		}
		row := tx.QueryRow(ctx, `
			INSERT INTO web_form (name, form_key, sender, allowed_origins, secret_ref, lead_origin, owner_id,
			  field_map, consent_required, consent_policy_version, consent_wording, redirect_url,
			  success_message, captured_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
			RETURNING `+formColumns,
			in.Name, key, in.Sender, in.AllowedOrigins, ref, in.LeadOrigin, in.OwnerID, in.FieldMap,
			in.ConsentRequired, in.ConsentPolicyVersion, in.ConsentWording, in.RedirectURL, in.SuccessMessage, by)
		if out, err = scanForm(row); err != nil {
			return err
		}
		_, err = storekit.Audit(ctx, tx, "create", auditEntity, out.ID, nil, auditImage(out))
		return err
	})
	if err != nil {
		s.dropSecret(ctx, ref, "web form create rolled back")
		return Form{}, "", err
	}
	return out, minted, nil
}

// checkReferences refuses a lead origin or owner the lead writer would refuse.
// A misconfigured form then fails when it is saved, not on a visitor's post.
func checkReferences(ctx context.Context, tx pgx.Tx, origin *string, owner *ids.UUID) error {
	if origin != nil {
		var known bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM lead_origin WHERE key = $1 AND active)`, *origin).Scan(&known); err != nil {
			return err
		}
		if !known {
			return badInput("lead_origin", "%q is not an active lead origin; list them at /v1/lead-origins", *origin)
		}
	}
	if owner != nil {
		return auth.EnsureNewRecordOwner(ctx, tx, *owner)
	}
	return nil
}

// auditImage is what the audit row records of a form: its configuration,
// never a secret or its ref.
func auditImage(f Form) map[string]any {
	return map[string]any{
		"name": f.Name, "sender": f.Sender, "allowed_origins": f.AllowedOrigins,
		"lead_origin": f.LeadOrigin, "owner_id": f.OwnerID, "field_map": f.FieldMap,
		"consent_required": f.ConsentRequired, "consent_policy_version": f.ConsentPolicyVersion,
		"redirect_url": f.RedirectURL, "state": f.State,
	}
}

// ListForms answers the workspace's forms, newest first.
func (s *Store) ListForms(ctx context.Context, archived storekit.ArchivedFilter) ([]Form, error) {
	if err := auth.Require(ctx, rbacObject, principal.ActionRead); err != nil {
		return nil, err
	}
	where := " WHERE archived_at IS NULL"
	if archived == storekit.IncludeArchived {
		where = ""
	}
	out := []Form{}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, "SELECT "+formColumns+" FROM web_form"+where+" ORDER BY created_at DESC, id DESC")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			f, err := scanForm(rows)
			if err != nil {
				return err
			}
			out = append(out, f)
		}
		return rows.Err()
	})
	return out, err
}

// GetForm answers one live form; an archived or absent one is ErrNotFound.
func (s *Store) GetForm(ctx context.Context, id ids.UUID) (Form, error) {
	if err := auth.Require(ctx, rbacObject, principal.ActionRead); err != nil {
		return Form{}, err
	}
	var out Form
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = scanForm(tx.QueryRow(ctx, "SELECT "+formColumns+" FROM web_form WHERE id = $1 AND archived_at IS NULL", id))
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Form{}, apperrors.ErrNotFound
	}
	return out, err
}

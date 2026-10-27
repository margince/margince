// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webforms

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// UpdateInput carries the fields a form can change. Its sender cannot: a form
// that changes how it is admitted is a different form, with another address.
// Clear names the nullable fields the request set to null.
type UpdateInput struct {
	Name                 *string
	AllowedOrigins       *[]string
	SenderSecret         *string
	LeadOrigin           *string
	OwnerID              *ids.UUID
	FieldMap             *map[string]string
	ConsentRequired      *bool
	ConsentPolicyVersion *string
	ConsentWording       *string
	RedirectURL          *string
	SuccessMessage       *string
	State                *string
	Clear                []string
	IfVersion            *int64
}

func clearableColumns(f Form) map[string]storekit.Clearable {
	return map[string]storekit.Clearable{
		"lead_origin":            {Column: "lead_origin", Current: f.LeadOrigin},
		"owner_id":               {Column: "owner_id", Current: f.OwnerID},
		"consent_policy_version": {Column: "consent_policy_version", Current: f.ConsentPolicyVersion},
		"consent_wording":        {Column: "consent_wording", Current: f.ConsentWording},
		"redirect_url":           {Column: "redirect_url", Current: f.RedirectURL},
		"success_message":        {Column: "success_message", Current: f.SuccessMessage},
	}
}

// merged answers the form as it reads once this update applies. A rule that
// spans two fields is checked against that result, not against the patch.
func (in UpdateInput) merged(f Form) (Form, error) {
	if in.Name != nil {
		name, err := checkName(*in.Name)
		if err != nil {
			return Form{}, err
		}
		f.Name = name
	}
	if in.AllowedOrigins != nil {
		origins, err := normalizeOrigins(*in.AllowedOrigins)
		if err != nil {
			return Form{}, err
		}
		f.AllowedOrigins = origins
	}
	assign(&f.LeadOrigin, in.LeadOrigin)
	assign(&f.OwnerID, in.OwnerID)
	assign(&f.ConsentPolicyVersion, in.ConsentPolicyVersion)
	assign(&f.ConsentWording, in.ConsentWording)
	assign(&f.RedirectURL, in.RedirectURL)
	assign(&f.SuccessMessage, in.SuccessMessage)
	if in.FieldMap != nil {
		f.FieldMap = *in.FieldMap
	}
	if in.ConsentRequired != nil {
		f.ConsentRequired = *in.ConsentRequired
	}
	if in.State != nil {
		if *in.State != StateActive && *in.State != StatePaused {
			return Form{}, badInput("state", "must be active or paused")
		}
		f.State = *in.State
	}
	for _, field := range in.Clear {
		clearField(&f, field)
	}
	return f, in.check(f)
}

func assign[T any](dst **T, v *T) {
	if v != nil {
		*dst = v
	}
}

func clearField(f *Form, field string) {
	switch field {
	case "lead_origin":
		f.LeadOrigin = nil
	case "owner_id":
		f.OwnerID = nil
	case "consent_policy_version":
		f.ConsentPolicyVersion = nil
	case "consent_wording":
		f.ConsentWording = nil
	case "redirect_url":
		f.RedirectURL = nil
	case "success_message":
		f.SuccessMessage = nil
	}
}

func (in UpdateInput) check(f Form) error {
	switch {
	case in.AllowedOrigins != nil && f.Sender != SenderBrowser:
		return badInput("allowed_origins", "only a browser form takes posts by website")
	case f.Sender == SenderBrowser && len(f.AllowedOrigins) == 0:
		return badInput("allowed_origins", "a browser form needs the website it is posted from")
	case in.SenderSecret != nil && !builderSender(f.Sender):
		return badInput("sender_secret", "only a builder form takes a pasted secret; a server form rotates its own")
	case in.SenderSecret != nil && *in.SenderSecret == "":
		return badInput("sender_secret", "must not be empty")
	}
	return checkSettings(f.FieldMap, f.ConsentRequired, f.ConsentWording, f.RedirectURL, f.SuccessMessage, in.SenderSecret)
}

// UpdateForm changes a form under the optimistic-concurrency guard. A new
// builder secret replaces the stored one, and the old one is destroyed once
// the row no longer points at it.
func (s *Store) UpdateForm(ctx context.Context, id ids.UUID, in UpdateInput) (Form, error) {
	if err := auth.Require(ctx, rbacObject, principal.ActionUpdate); err != nil {
		return Form{}, err
	}
	newRef, err := s.replacementSecret(ctx, id, in.SenderSecret)
	if err != nil {
		return Form{}, err
	}
	var out Form
	var replacedRef *string
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		lock, err := storekit.LockRow(ctx, tx, "web_form", id, storekit.LiveOnly)
		if err != nil {
			return err
		}
		if err := storekit.EnsureIfMatch(ctx, tx, "web_form", id, in.IfVersion); err != nil {
			return err
		}
		current, err := scanForm(tx.QueryRow(ctx, "SELECT "+formColumns+" FROM web_form WHERE id = $1", id))
		if err != nil {
			return err
		}
		next, err := in.merged(current)
		if err != nil {
			return err
		}
		if err := checkReferences(ctx, tx, in.LeadOrigin, in.OwnerID); err != nil {
			return err
		}
		p := patchFor(current, next)
		if err := storekit.ApplyClears(p, in.Clear, clearableColumns(current)); err != nil {
			return err
		}
		if newRef != nil {
			p.Set("secret_ref", current.SecretRef, newRef)
			replacedRef = current.SecretRef
		}
		if out = current; p.Empty() {
			return nil
		}
		if err := p.ApplyLocked(ctx, tx, lock); err != nil {
			return err
		}
		if _, err := storekit.Audit(ctx, tx, "update", auditEntity, id, auditImage(current), auditImage(next)); err != nil {
			return err
		}
		out, err = scanForm(tx.QueryRow(ctx, "SELECT "+formColumns+" FROM web_form WHERE id = $1", id))
		return err
	})
	if err != nil {
		s.dropSecret(ctx, newRef, "web form secret update rolled back")
		return Form{}, err
	}
	s.dropSecret(ctx, replacedRef, "web form secret replaced")
	return out, nil
}

// replacementSecret seals a builder's new secret before the update's
// transaction opens. The vault writes through a connection of its own. Taking
// one while holding another is how a busy pool deadlocks.
func (s *Store) replacementSecret(ctx context.Context, id ids.UUID, pasted *string) (*string, error) {
	if pasted == nil {
		return nil, nil
	}
	current, err := s.GetForm(ctx, id)
	if err != nil {
		return nil, err
	}
	if !builderSender(current.Sender) || *pasted == "" {
		return nil, UpdateInput{SenderSecret: pasted}.check(current)
	}
	ref, _, err := s.storeSecret(ctx, current.Sender, pasted)
	return ref, err
}

// patchFor records every column the merge moved. The secret ref is set by the
// caller, because only it knows the new ref.
func patchFor(current, next Form) *storekit.Patch {
	p := storekit.NewPatch()
	p.Set("name", current.Name, next.Name)
	p.Set("allowed_origins", current.AllowedOrigins, next.AllowedOrigins)
	p.Set("field_map", current.FieldMap, next.FieldMap)
	p.Set("consent_required", current.ConsentRequired, next.ConsentRequired)
	p.Set("state", current.State, next.State)
	for column, pair := range map[string][2]*string{
		"lead_origin":            {current.LeadOrigin, next.LeadOrigin},
		"consent_policy_version": {current.ConsentPolicyVersion, next.ConsentPolicyVersion},
		"consent_wording":        {current.ConsentWording, next.ConsentWording},
		"redirect_url":           {current.RedirectURL, next.RedirectURL},
		"success_message":        {current.SuccessMessage, next.SuccessMessage},
	} {
		if pair[1] != nil {
			p.Set(column, pair[0], pair[1])
		}
	}
	if next.OwnerID != nil {
		p.Set("owner_id", current.OwnerID, next.OwnerID)
	}
	return p
}

// RotateSecret mints a new secret for a server form and answers it once. The
// old one stops verifying the moment this commits.
func (s *Store) RotateSecret(ctx context.Context, id ids.UUID) (Form, string, error) {
	if err := auth.Require(ctx, rbacObject, principal.ActionUpdate); err != nil {
		return Form{}, "", err
	}
	current, err := s.GetForm(ctx, id)
	if err != nil {
		return Form{}, "", err
	}
	if current.Sender != SenderServer {
		return Form{}, "", badInput("sender", "only a server form has a secret this installation mints; paste a builder's new secret as sender_secret")
	}
	newRef, secret, err := s.storeSecret(ctx, current.Sender, nil)
	if err != nil {
		return Form{}, "", err
	}
	var out Form
	var oldRef *string
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			WITH old AS (SELECT secret_ref FROM web_form WHERE id = $1 AND archived_at IS NULL FOR UPDATE)
			UPDATE web_form w SET secret_ref = $2 FROM old WHERE w.id = $1
			RETURNING old.secret_ref`, id, newRef).Scan(&oldRef)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		} else if err != nil {
			return err
		}
		if _, err := storekit.AuditEvent(ctx, tx, "update", auditEntity, id, map[string]any{"signing_secret_rotated": true}); err != nil {
			return err
		}
		out, err = scanForm(tx.QueryRow(ctx, "SELECT "+formColumns+" FROM web_form WHERE id = $1", id))
		return err
	})
	if err != nil {
		s.dropSecret(ctx, newRef, "web form rotation rolled back")
		return Form{}, "", err
	}
	s.dropSecret(ctx, oldRef, "web form secret rotated")
	return out, secret, nil
}

// ArchiveForm retires a form: its address answers 404 from then on, and its
// secret is destroyed, since nothing can verify against it again.
func (s *Store) ArchiveForm(ctx context.Context, id ids.UUID) (Form, error) {
	if err := auth.Require(ctx, rbacObject, principal.ActionDelete); err != nil {
		return Form{}, err
	}
	var out Form
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = scanForm(tx.QueryRow(ctx,
			"UPDATE web_form SET archived_at = now() WHERE id = $1 AND archived_at IS NULL RETURNING "+formColumns, id))
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		} else if err != nil {
			return err
		}
		_, err = storekit.Audit(ctx, tx, "archive", auditEntity, id, nil, nil)
		return err
	})
	if err != nil {
		return Form{}, err
	}
	s.dropSecret(ctx, out.SecretRef, "web form archived")
	return out, nil
}

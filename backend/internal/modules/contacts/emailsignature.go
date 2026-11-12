// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Personal signatures stay owner-only while their member is active.
// A departed member's retained signature has a separate controller review surface.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/signature"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// SignatureMaxRunes bounds a signature. What one is FOR is a name, a role and a
// way to reach the sender; past this it is a document riding on every message
// the contact sends.
const SignatureMaxRunes = 2000

// SignatureFieldMaxRunes bounds the title and the phone a template fills in.
const SignatureFieldMaxRunes = 120

// SignatureTemplateMaxRunes bounds the workspace's signature template.
const SignatureTemplateMaxRunes = 4000

// SignatureTemplate is the workspace layout used after personal HTML.
var SignatureTemplate = config.DefineStored(config.Spec[string]{
	Key:  "contacts.signature_template",
	Env:  "MARGINCE_CONTACTS_SIGNATURE_TEMPLATE",
	Type: config.String,
	Validate: func(template string) error {
		if len([]rune(template)) > SignatureTemplateMaxRunes {
			return fmt.Errorf("the template is at most %d characters", SignatureTemplateMaxRunes)
		}
		return nil
	},
	Doc: "Workspace email signature template, used when the sender has no personal HTML; empty means none.",
}, config.Store{
	RBACObject: "installation_settings", AuditVerb: settingsAuditVerb,
	MachineryApplied: true, // the send path applies it to the sender's own sign-off
})

// EmailSignature is the caller's sign-off as the settings tab shows it.
type EmailSignature struct {
	// Body is the plain fallback after personal HTML and the workspace template.
	Body string
	HTML string
	// Title and Phone fill the workspace template's placeholders.
	Title, Phone string
	// TemplateActive says the caller's mail has a template for when they have no
	// personal HTML: their legal entity's own, or the workspace's.
	TemplateActive bool
	UpdatedAt      *time.Time
}

// SaveSignatureInput is what the caller writes about their own sign-off.
// A nil optional field leaves its stored value unchanged.
type SaveSignatureInput struct {
	Body               string
	Title, Phone, HTML *string
}

// SenderSignature is what a send needs to sign as the caller. That is their own
// text and markup, their template values, and their legal entity's template.
type SenderSignature struct {
	Body, HTML, Title, Phone, Template string
	// Imprint is the legal notice of the sender's entity, empty when it has none.
	Imprint string
	// LogoKey is the stored object of the sender's entity logo, empty when it
	// has none. A send stages this key, so a retry embeds the same picture.
	LogoKey string
}

// signingEntity is what the legal entity a message goes out for gives it: its
// template, falling back to the workspace's, and its imprint and logo. A zero
// entity is the one the user works for now.
type signingEntity struct {
	template, imprint string
	// logoKey is set only on a further entity. The default entity wears the
	// company logo, which the send reads through the company grant.
	logoKey   string
	isDefault bool
}

func signingEntityTx(ctx context.Context, tx pgx.Tx, user, sending ids.UUID) (signingEntity, error) {
	workspace, err := config.StoredApplyTx(ctx, tx, config.Bound(), SignatureTemplate)
	if err != nil {
		return signingEntity{}, err
	}
	entityID := ids.From[ids.LegalEntityKind](sending)
	if sending.IsZero() {
		if entityID, err = UserLegalEntity(ctx, tx, ids.From[ids.UserKind](user)); err != nil {
			return signingEntity{}, err
		}
	}
	entity, err := ResolveEntityIdentity(ctx, tx, entityID)
	if err != nil {
		return signingEntity{}, err
	}
	out := signingEntity{template: workspace, imprint: deref(entity.Imprint), isDefault: entity.IsDefault}
	if own := strings.TrimSpace(deref(entity.SignatureTemplate)); own != "" {
		out.template = own
	}
	if !entity.IsDefault {
		out.logoKey = deref(entity.LogoObjectKey)
	}
	return out, nil
}

// GetMyEmailSignature reads the caller's own signature. A member who has never
// written one has no row, and that is not an error: an empty body is the honest
// answer, and the send path closes with a plain greeting and the member's name.
func (s *Store) GetMyEmailSignature(ctx context.Context) (EmailSignature, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == ids.Nil {
		return EmailSignature{}, apperrors.ErrPermissionDenied
	}
	var out EmailSignature
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		entity, err := signingEntityTx(ctx, tx, actor.UserID, ids.Nil)
		if err != nil {
			return err
		}
		out.TemplateActive = strings.TrimSpace(entity.template) != ""
		err = tx.QueryRow(ctx, `
			SELECT body, html, title, phone, updated_at FROM email_signature
			 WHERE owner_id = $1 AND archived_at IS NULL`, actor.UserID).
			Scan(&out.Body, &out.HTML, &out.Title, &out.Phone, &out.UpdatedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return EmailSignature{}, fmt.Errorf("contacts: reading the caller's email signature: %w", err)
	}
	return out, nil
}

// SaveMyEmailSignature saves each supplied field; an omitted optional field keeps its value.
func (s *Store) SaveMyEmailSignature(ctx context.Context, in SaveSignatureInput) (EmailSignature, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == ids.Nil {
		return EmailSignature{}, apperrors.ErrPermissionDenied
	}
	in, err := validatedSignature(in)
	if err != nil {
		return EmailSignature{}, err
	}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := lockSignatureWriterTx(ctx, tx, actor.UserID); err != nil {
			return err
		}
		before, err := readSignatureTx(ctx, tx, actor.UserID)
		if err != nil {
			return err
		}
		return saveSignatureTx(ctx, tx, actor.UserID, before, in, true)
	})
	if err != nil {
		return EmailSignature{}, fmt.Errorf("contacts: saving the caller's email signature: %w", err)
	}
	return s.GetMyEmailSignature(ctx)
}

func upsertSignatureTx(ctx context.Context, tx pgx.Tx, userID ids.UUID, body string, title, phone, markup *string, restore bool) error {
	_, err := tx.Exec(ctx, `INSERT INTO email_signature (owner_id, body, title, phone, html)
 VALUES (@owner,@body,coalesce(@title::text,''),coalesce(@phone::text,''),coalesce(@html::text,''))
 ON CONFLICT (owner_id) DO UPDATE SET body=EXCLUDED.body,
 title=coalesce(@title::text,email_signature.title), phone=coalesce(@phone::text,email_signature.phone),
 html=coalesce(@html::text,email_signature.html),
 archived_at=CASE WHEN @restore THEN NULL ELSE email_signature.archived_at END`,
		pgx.NamedArgs{signatureOwnerArg: userID, "body": body, fieldTitle: title, fieldPhone: phone, "html": markup, "restore": restore})
	return err
}

// SignatureFor reads one user's signature for the send path, for the legal
// entity named by sending. Zero is the entity the user works for now.
//
// Separate from the Get above because the caller is different: that one answers
// a settings tab about the reader reading it, this one answers the mailer about
// the contact SENDING. Both resolve the same row; neither lets one member read
// another's, because the send path only ever asks about its own authenticated
// sender.
func (s *Store) SignatureFor(ctx context.Context, userID, sending ids.UUID) (SenderSignature, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID != userID {
		// A send signs with the SENDER's own sign-off. Asking for anybody
		// else's is a bug in the caller, not a permission to widen.
		return SenderSignature{}, apperrors.ErrPermissionDenied
	}
	var out SenderSignature
	var entity signingEntity
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		entity, err = signingEntityTx(ctx, tx, userID, sending)
		if err != nil {
			return err
		}
		out.Template, out.Imprint = entity.template, entity.imprint
		err = tx.QueryRow(ctx, `
			SELECT body, html, title, phone FROM email_signature
			 WHERE owner_id = $1 AND archived_at IS NULL`, userID).Scan(&out.Body, &out.HTML, &out.Title, &out.Phone)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return SenderSignature{}, fmt.Errorf("contacts: reading the sender's email signature: %w", err)
	}
	// A further entity's logo is its own, or none: it never borrows the
	// company's. Read even with no template stored, so a preview of an unsaved
	// one shows the logo too.
	if !entity.isDefault {
		out.LogoKey = entity.logoKey
		return out, nil
	}
	// A seat that may not read companies signs without the company logo.
	out.LogoKey, err = s.AnchorLogoKey(ctx)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return out, nil
	}
	return out, err
}

func readSignatureTx(ctx context.Context, tx pgx.Tx, userID ids.UUID) (EmailSignature, error) {
	var stored EmailSignature
	err := tx.QueryRow(ctx, `
		SELECT body, html FROM email_signature
		 WHERE owner_id = $1 AND archived_at IS NULL`, userID).Scan(&stored.Body, &stored.HTML)
	if errors.Is(err, pgx.ErrNoRows) {
		return stored, nil
	}
	return stored, err
}

func savedHTML(stored string, updated *string) string {
	if updated != nil {
		return *updated
	}
	return stored
}

// SignatureTooLongError is the shared saved-field and draft refusal.
type SignatureTooLongError = signature.TooLongError

// trimmedField trims a field the caller sent and keeps an omitted one nil.
func trimmedField(field *string) *string {
	if field == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*field)
	return &trimmed
}

func fieldRunes(field *string) int {
	if field == nil {
		return 0
	}
	return len([]rune(*field))
}

const signatureOwnerArg = "owner"

func lockSignatureWriterTx(ctx context.Context, tx pgx.Tx, user ids.UUID) error {
	// The identity lock also serializes first saves, before a signature row exists.
	if err := storekit.LockWriteIdentity(ctx, tx, "email_signature", user.String()); err != nil {
		return err
	}
	var live bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM app_user WHERE id=@owner AND `+workforce.LiveMemberSQL("")+`)`, pgx.NamedArgs{signatureOwnerArg: user}).Scan(&live); err != nil {
		return err
	}
	if !live {
		return apperrors.ErrNotFound
	}
	return nil
}

func validatedSignature(in SaveSignatureInput) (SaveSignatureInput, error) {
	trimmed := strings.TrimSpace(in.Body)
	if len([]rune(trimmed)) > SignatureMaxRunes {
		return SaveSignatureInput{}, &SignatureTooLongError{Field: "body", Max: SignatureMaxRunes, Runes: len([]rune(trimmed))}
	}
	title, phone, markup := trimmedField(in.Title), trimmedField(in.Phone), trimmedField(in.HTML)
	if markup != nil {
		if err := signature.ValidateHTML(*markup); err != nil {
			return SaveSignatureInput{}, err
		}
	}
	for _, field := range []struct {
		name  string
		value *string
	}{{fieldTitle, title}, {fieldPhone, phone}} {
		if runes := fieldRunes(field.value); runes > SignatureFieldMaxRunes {
			return SaveSignatureInput{}, &SignatureTooLongError{Field: field.name, Max: SignatureFieldMaxRunes, Runes: runes}
		}
	}

	return SaveSignatureInput{Body: trimmed, Title: title, Phone: phone, HTML: markup}, nil
}

func saveSignatureTx(ctx context.Context, tx pgx.Tx, owner ids.UUID, before EmailSignature, in SaveSignatureInput, restore bool) error {
	if err := upsertSignatureTx(ctx, tx, owner, in.Body, in.Title, in.Phone, in.HTML, restore); err != nil {
		return err
	}
	// Audit and events record the change without copying personal signature text.
	auditID, err := storekit.Audit(ctx, tx, "update", "user", owner,
		map[string]any{"has_signature": before.Body != "", "has_html_signature": before.HTML != ""},
		map[string]any{"has_signature": in.Body != "", "has_html_signature": savedHTML(before.HTML, in.HTML) != ""})
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, owner,
		crmcontracts.PublicEventEmailSignatureChanged{HasSignature: in.Body != "" || savedHTML(before.HTML, in.HTML) != ""})
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The sender's own sign-off (core 0235).
//
// Always the CALLER's, never anybody else's, and there is no seat that widens
// that — not admin, not ops. A signature is the words a contact signs their name
// with, and no role in this product has a reason to read or rewrite another
// member's. The gate is therefore the actor themselves rather than an RBAC
// object: `owner_id = the caller` is the whole rule, applied in every statement.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SignatureMaxRunes bounds a signature. What one is FOR is a name, a role and a
// way to reach the sender; past this it is a document riding on every message
// the contact sends.
const SignatureMaxRunes = 2000

// SignatureFieldMaxRunes bounds the title and the phone a template fills in.
const SignatureFieldMaxRunes = 120

// SignatureTemplateMaxRunes bounds the workspace's signature template.
const SignatureTemplateMaxRunes = 4000

// SignatureTemplate is the workspace's signature layout, set by an admin. Empty
// means none: each sender signs with their own plain-text signature.
var SignatureTemplate = settings.Define[string](
	"contacts.signature_template", "installation_settings", "update", "",
	func(template string) error {
		if len([]rune(template)) > SignatureTemplateMaxRunes {
			return fmt.Errorf("the template is at most %d characters", SignatureTemplateMaxRunes)
		}
		return nil
	},
).MachineryApplied() // the send path applies it to the sender's own sign-off

// EmailSignature is the caller's sign-off as the settings tab shows it.
type EmailSignature struct {
	// Body empty means none written; the send path then closes with a plain
	// greeting and the member's name instead.
	Body string
	// Title and Phone fill the workspace template's placeholders.
	Title, Phone string
	// TemplateActive says the workspace has a template, which then replaces
	// Body on every send.
	TemplateActive bool
	UpdatedAt      *time.Time
}

// SaveSignatureInput is what the caller writes about their own sign-off.
// A nil Title or Phone leaves the stored value as it is.
type SaveSignatureInput struct {
	Body         string
	Title, Phone *string
}

// SenderSignature is what a send needs to sign as the caller: their own text,
// their template values, and the workspace's template.
type SenderSignature struct {
	Body, Title, Phone, Template string
	// LogoKey is the stored object of the workspace's own logo, empty when it
	// has none. A send stages this key, so a retry embeds the same picture.
	LogoKey string
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
		template, err := settings.ApplyTx(ctx, tx, SignatureTemplate)
		if err != nil {
			return err
		}
		out.TemplateActive = strings.TrimSpace(template) != ""
		err = tx.QueryRow(ctx, `
			SELECT body, title, phone, updated_at FROM email_signature
			 WHERE owner_id = $1 AND archived_at IS NULL`, actor.UserID).
			Scan(&out.Body, &out.Title, &out.Phone, &out.UpdatedAt)
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

// SaveMyEmailSignature upserts the caller's own row.
//
// An empty body CLEARS it: a member emptying the field means "sign off with
// the plain closing", not "leave what was there". The row survives the clearing so the
// audit trail keeps both sides of the change.
func (s *Store) SaveMyEmailSignature(ctx context.Context, in SaveSignatureInput) (EmailSignature, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == ids.Nil {
		return EmailSignature{}, apperrors.ErrPermissionDenied
	}
	trimmed := strings.TrimSpace(in.Body)
	if len([]rune(trimmed)) > SignatureMaxRunes {
		return EmailSignature{}, &SignatureTooLongError{Runes: len([]rune(trimmed))}
	}
	title, phone := trimmedField(in.Title), trimmedField(in.Phone)
	if longest := max(fieldRunes(title), fieldRunes(phone)); longest > SignatureFieldMaxRunes {
		return EmailSignature{}, &SignatureTooLongError{Runes: longest}
	}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		before, err := readSignatureTx(ctx, tx, actor.UserID)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO email_signature (owner_id, body, title, phone)
			VALUES ($1, $2, coalesce($3::text, ''), coalesce($4::text, ''))
			ON CONFLICT (owner_id) DO UPDATE SET body = $2,
			  title = coalesce($3::text, email_signature.title),
			  phone = coalesce($4::text, email_signature.phone), archived_at = NULL`,
			actor.UserID, trimmed, title, phone); err != nil {
			return err
		}
		// A signature goes out under the sender's name on every message they
		// send, so a change to it is a change to how they are represented —
		// exactly the kind of fact that has to be answerable later.
		//
		// The TEXT stays out of the audit payload and out of the event: it is
		// the member's own words about themselves, a reader needs to know the
		// sign-off changed rather than what somebody's home address is, and the
		// audit log is read by more contacts than the signature is.
		auditID, err := storekit.Audit(ctx, tx, "update", "user", actor.UserID,
			map[string]any{"has_signature": before != ""},
			map[string]any{"has_signature": trimmed != ""})
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, auditID, actor.UserID,
			crmcontracts.PublicEventEmailSignatureChanged{HasSignature: trimmed != ""})
	})
	if err != nil {
		return EmailSignature{}, fmt.Errorf("contacts: saving the caller's email signature: %w", err)
	}
	return s.GetMyEmailSignature(ctx)
}

// SignatureFor reads one user's signature for the send path.
//
// Separate from the Get above because the caller is different: that one answers
// a settings tab about the reader reading it, this one answers the mailer about
// the contact SENDING. Both resolve the same row; neither lets one member read
// another's, because the send path only ever asks about its own authenticated
// sender.
func (s *Store) SignatureFor(ctx context.Context, userID ids.UUID) (SenderSignature, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID != userID {
		// A send signs with the SENDER's own sign-off. Asking for anybody
		// else's is a bug in the caller, not a permission to widen.
		return SenderSignature{}, apperrors.ErrPermissionDenied
	}
	var out SenderSignature
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		template, err := settings.ApplyTx(ctx, tx, SignatureTemplate)
		if err != nil {
			return err
		}
		out.Template = template
		err = tx.QueryRow(ctx, `
			SELECT body, title, phone FROM email_signature
			 WHERE owner_id = $1 AND archived_at IS NULL`, userID).Scan(&out.Body, &out.Title, &out.Phone)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	if err != nil {
		return SenderSignature{}, fmt.Errorf("contacts: reading the sender's email signature: %w", err)
	}
	// Read even with no template stored, so a preview of an unsaved one shows
	// the logo too. A seat that may not read companies signs without it.
	out.LogoKey, err = s.AnchorLogoKey(ctx)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return out, nil
	}
	return out, err
}

func readSignatureTx(ctx context.Context, tx pgx.Tx, userID ids.UUID) (string, error) {
	var body string
	err := tx.QueryRow(ctx, `
		SELECT body FROM email_signature
		 WHERE owner_id = $1 AND archived_at IS NULL`, userID).Scan(&body)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return body, err
}

// SignatureTooLongError names the field and the bar, because a refusal that
// only says "too long" leaves the member counting characters by hand.
type SignatureTooLongError struct{ Runes int }

func (e *SignatureTooLongError) Error() string {
	return fmt.Sprintf("contacts: a signature is at most %d characters, this one is %d",
		SignatureMaxRunes, e.Runes)
}

// FieldFault names the offending input for EVERY surface, not just this
// module's HTTP transport. The MCP tool surface reaches this code through the
// datasource seam and never through that transport, so a refusal expressed only
// as a transport branch would reach an agent as an internal fault it was told
// to retry — retrying a signature that is too long forever.
func (e *SignatureTooLongError) FieldFault() (field, code, message string) {
	return "body", "too_long", e.Error()
}

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

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The wording a proof row points at, published once and never edited.
//
// consent_event stores the sentence a subject was shown, verbatim, which is
// what makes a proof row honest. It does not make the wording REPRODUCIBLE: the
// copy on the event cannot show what else that version disclosed alongside the
// sentence, and cannot be told from a copy somebody edited afterwards. A
// published row is the canonical text both questions resolve against.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// TextVersion is one published wording.
type TextVersion struct {
	Key     string
	Version string
	// Locale is which language this text is, empty for a wording that applies
	// in every language. A controller template names one: the same template at
	// the same version has an English, a German and a Vietnamese text, and a
	// proof row has to say which of the three a contact was shown.
	Locale      string
	Subject     string
	Body        string
	PublishedAt time.Time
}

// describe names one wording for an error message: key@version, and the
// language when it names one.
func (in TextVersion) describe() string {
	if in.Locale == "" {
		return in.Key + "@" + in.Version
	}
	return in.Key + "@" + in.Version + " (" + in.Locale + ")"
}

// localeOrNil turns an unset locale into the NULL the column means by it: this
// wording applies in every language.
func localeOrNil(locale string) *string {
	if locale == "" {
		return nil
	}
	return &locale
}

// WordingDigest is the ONE way a subject line and a body are reduced to a
// digest in this module.
//
// Separated by a NUL so a sentence moved from one field to the other is a
// different digest rather than the same one. The send path's content
// fingerprint (authorizestaging.go, authorizetransmit.go) and this table's
// content hash are the same question asked of the same two strings; they differ
// only in how they STORE the answer — bytea there, hex text here — so the
// digest is computed once and each caller renders it.
//
// Held by: TestOneWordingDigest (backend/gates/wordingdigest_test.go), which
// fails when a second sha256 over a subject-and-body pair appears in the module.
func WordingDigest(subject, body string) [sha256.Size]byte {
	return sha256.Sum256([]byte(subject + "\x00" + body))
}

// SendingDigest is the digest over everything a RECIPIENT can read: the
// subject, the plain-text alternative, and the markup one.
//
// The markup is in it because a mail carries two bodies and the tree says so
// itself — a client rendering markup can show something the plain part never
// mentioned. A fingerprint over the plain body alone would let the half most
// recipients actually read be rewritten between the decision and the send while
// the record went on claiming the message was unchanged.
//
// NUL-separated, and three fields rather than two so a sentence moved from one
// to another is a different digest.
//
// IT IS NOT WordingDigest WITH AN EMPTY THIRD ARGUMENT, and that is deliberate
// rather than a missed simplification. Appending a separator changes the hash
// of every plain-text message: `S\x00B` and `S\x00B\x00` are different digests.
// Folding the two would silently invalidate every consent_text_version row
// already published — bootstrap re-publishes the controller templates and would
// find unchanged wording conflicting, which fails startup — and would park
// every delivery staged before the deploy, because its stored fingerprint would
// no longer match its own unchanged body.
//
// So the two-field digest keeps its exact bytes and this is a separate
// question: what a RECIPIENT can read, which a stored wording has no markup
// half of. The wordingdigest gate holds that neither grows a third spelling.
func SendingDigest(subject, body, htmlBody string) [sha256.Size]byte {
	return sha256.Sum256([]byte(subject + "\x00" + body + "\x00" + htmlBody))
}

// ContentHashOf renders the digest as the hex text consent_text_version stores.
func ContentHashOf(subject, body string) string {
	sum := WordingDigest(subject, body)
	return hex.EncodeToString(sum[:])
}

// fieldKey is the name of a wording's key wherever it crosses a boundary: the
// validation field a caller reads, and the audit payload a reviewer reads.
const fieldKey = "key"

// PublishTextVersionTx records one wording as published.
//
// IDEMPOTENT on key+version: the catalog publishes on every boot, so a second
// call with the same wording must be a no-op rather than a conflict. It is NOT
// idempotent on the TEXT — republishing a key+version whose body has changed is
// refused, because that is the edit the trigger exists to prevent arriving by
// another door.
func PublishTextVersionTx(ctx context.Context, tx pgx.Tx, in TextVersion) (ids.UUID, error) {
	if in.Key == "" || in.Version == "" {
		return ids.UUID{}, &ValidationError{
			Field:  fieldKey,
			Reason: "a published wording names which text it is and which version",
		}
	}
	if in.Body == "" {
		return ids.UUID{}, &ValidationError{
			Field:  "body",
			Reason: "a version with no body names nothing a proof row could point at",
		}
	}
	hash := ContentHashOf(in.Subject, in.Body)

	var existingID ids.UUID
	var existingHash string
	// Read by locale as well as by key and version, matching the published
	// index. Asked without it, the second language of one template would find
	// the first, compare a German body against an English hash and refuse the
	// boot as a wording change.
	err := tx.QueryRow(ctx, `
		SELECT id, content_hash FROM consent_text_version
		 WHERE key = $1 AND version = $2
		   AND locale IS NOT DISTINCT FROM $3 AND published_at IS NOT NULL`,
		in.Key, in.Version, localeOrNil(in.Locale)).Scan(&existingID, &existingHash)
	switch {
	case err == nil:
		if existingHash != hash {
			return ids.UUID{}, fmt.Errorf(
				"%s is published with different wording; publish a new version rather than "+
					"changing what this one said: %w", in.describe(), apperrors.ErrConflict)
		}
		return existingID, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return ids.UUID{}, fmt.Errorf("read the published wording for %s: %w", in.describe(), err)
	}

	var id ids.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO consent_text_version (key, version, locale, subject_line, body, content_hash, published_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id`,
		in.Key, in.Version, localeOrNil(in.Locale), nullableText(in.Subject),
		in.Body, hash, in.PublishedAt).Scan(&id); err != nil {
		return ids.UUID{}, fmt.Errorf("publish the wording %s: %w", in.describe(), err)
	}
	if _, err := storekit.Audit(ctx, tx, "create", "consent_text_version", id, nil, map[string]any{
		fieldKey: in.Key, "version": in.Version, "locale": in.Locale, "content_hash": hash,
	}); err != nil {
		return ids.UUID{}, fmt.Errorf("audit the published wording: %w", err)
	}
	return id, nil
}

// PublishControllerTemplatesTx publishes the installation's own wording, so a
// proof row and a controller mail can both name the version they came from.
//
// Called at bootstrap beside the purpose catalog, and idempotent: it runs on
// every boot, finds each version already published, and returns. A template
// whose body changed without its version moving is REFUSED here — that is the
// edit the immutability trigger forbids, arriving one call further out, and it
// is worth failing a boot over: the alternative is a live version whose text no
// longer matches the proofs that name it.
//
// The wording is published WITHOUT an expiry date, which is what makes it the
// canonical text rather than one message's render: the date belongs to a
// particular link, and publishing a rendered copy would name one send's wording
// as though it were the template's.
//
// EVERY LANGUAGE THIS BUILD SPEAKS is published, not just the installation's
// own. The installation's language can change after a link was sent, so a
// wording published only for the setting live at boot would disappear from a
// later boot — and the text somebody was shown in March would be gone in
// September. Publishing all three costs two extra rows per template and keeps
// each of them answerable.
//
// WHAT IT DOES NOT YET DO. consent_event.consent_text_version_id has no writer
// anywhere in this tree, so nothing records WHICH of the three a given proof
// was rendered from. This change makes all three resolvable; picking one is the
// writer's job and that writer does not exist. When it lands it must take the
// locale from the language the mail was actually staged in, not from
// installation.base_language at read time, which is the value that can have
// changed in between.
func PublishControllerTemplatesTx(ctx context.Context, tx pgx.Tx, now time.Time) error {
	for key, t := range controllerTemplates {
		for _, language := range mailcopy.Languages() {
			rendered, _, err := RenderControllerTemplate(key, time.Time{}, string(language))
			if err != nil {
				return err
			}
			if _, err := PublishTextVersionTx(ctx, tx, TextVersion{
				Key:         key,
				Version:     fmt.Sprintf("v%d", t.version),
				Locale:      string(language),
				Subject:     rendered.Subject,
				Body:        rendered.Body,
				PublishedAt: now,
			}); err != nil {
				return err
			}
		}
	}
	return nil
}

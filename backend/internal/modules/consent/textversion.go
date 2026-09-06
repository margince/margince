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
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// TextVersion is one published wording.
type TextVersion struct {
	Key         string
	Version     string
	Subject     string
	Body        string
	PublishedAt time.Time
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
	err := tx.QueryRow(ctx, `
		SELECT id, content_hash FROM consent_text_version
		 WHERE key = $1 AND version = $2 AND published_at IS NOT NULL`,
		in.Key, in.Version).Scan(&existingID, &existingHash)
	switch {
	case err == nil:
		if existingHash != hash {
			return ids.UUID{}, fmt.Errorf(
				"%s@%s is published with different wording; publish a new version rather than "+
					"changing what this one said: %w", in.Key, in.Version, apperrors.ErrConflict)
		}
		return existingID, nil
	case !errors.Is(err, pgx.ErrNoRows):
		return ids.UUID{}, fmt.Errorf("read the published wording for %s@%s: %w", in.Key, in.Version, err)
	}

	var id ids.UUID
	if err := tx.QueryRow(ctx, `
		INSERT INTO consent_text_version (key, version, subject_line, body, content_hash, published_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id`,
		in.Key, in.Version, nullableText(in.Subject), in.Body, hash, in.PublishedAt).Scan(&id); err != nil {
		return ids.UUID{}, fmt.Errorf("publish the wording %s@%s: %w", in.Key, in.Version, err)
	}
	if _, err := storekit.Audit(ctx, tx, "create", "consent_text_version", id, nil, map[string]any{
		fieldKey: in.Key, "version": in.Version, "content_hash": hash,
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
// The template's own body is published, not one render of it. RenderControllerTemplate
// substitutes an expiry date, so a rendered copy would publish one message's
// wording as though it were the canonical text.
func PublishControllerTemplatesTx(ctx context.Context, tx pgx.Tx, now time.Time) error {
	for key, t := range controllerTemplates {
		if _, err := PublishTextVersionTx(ctx, tx, TextVersion{
			Key:         key,
			Version:     fmt.Sprintf("v%d", t.version),
			Subject:     t.subject,
			Body:        t.body,
			PublishedAt: now,
		}); err != nil {
			return err
		}
	}
	return nil
}

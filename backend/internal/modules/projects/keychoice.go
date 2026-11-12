// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

// Which keys a project may carry. A chosen key and a suggested key pass one
// refusal, so the form never suggests a key it would refuse.

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// projectKeyShape is the project_key_shape CHECK in Go, so a chosen key is
// refused with a reason before the INSERT rather than by the constraint net.
var projectKeyShape = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{1,23}$`)

// subjectWords are words senders put in square brackets for reasons of their
// own. They are reply and forward markers, gateway tags and everyday labels.
// A key spelled as one of them would file every such mail under the project.
var subjectWords = map[string]bool{
	"re": true, "fw": true, "fwd": true, "aw": true, "wg": true, "sv": true, "tr": true,
	"tl": true, "ext": true, "extern": true, "external": true, "internal": true,
	"intern": true, "spam": true, "junk": true, "suspicious": true, "phishing": true,
	"warning": true, "caution": true, "secure": true, "encrypt": true, "encrypted": true,
	"confidential": true, "vertraulich": true, "private": true, "privat": true,
	"public": true, "urgent": true, "dringend": true, "important": true, "wichtig": true,
	"fyi": true, "update": true, "status": true, "notice": true, "alert": true, "reminder": true, "action": true,
	"todo": true, "draft": true, "entwurf": true, "test": true, "request": true,
	"question": true, "frage": true, "offer": true, "angebot": true, "invoice": true,
	"rechnung": true, "order": true, "auftrag": true, "contract": true, "vertrag": true,
	"meeting": true, "termin": true, "call": true, "project": true, "projekt": true,
	"new": true, "neu": true, "ok": true, "yes": true, "no": true, "ja": true,
	"nein": true, "the": true, "and": true, "for": true, "und": true, "der": true,
	"die": true, "das": true, "mit": true, "von": true,
}

// ProjectKeyRefusedError maps to 422: a key this project may not carry, with
// the reason the creator can act on.
type ProjectKeyRefusedError struct {
	Code    string
	Message string
}

func (e *ProjectKeyRefusedError) Error() string { return e.Message }

// FieldFault places the refusal on the key field of the New project form.
func (e *ProjectKeyRefusedError) FieldFault() (field, code, message string) {
	return projectKeyField, e.Code, e.Message
}

func keyTaken(key string) error {
	return &ProjectKeyRefusedError{
		Code:    "key_taken",
		Message: fmt.Sprintf("another live project already has the key %s; choose another key", key),
	}
}

// normalizeChosenKey answers the key as stored: trimmed and upper case, the
// spelling every minted key has, so two projects never differ by case alone.
func normalizeChosenKey(raw string) (string, error) {
	key := strings.ToUpper(strings.TrimSpace(raw))
	if !projectKeyShape.MatchString(key) {
		return "", &ProjectKeyShapeError{}
	}
	return key, nil
}

// refuseChosenKey holds a typed key to the word refusal and to uniqueness
// among live projects. normalizeChosenKey has already checked its shape.
func refuseChosenKey(ctx context.Context, tx pgx.Tx, key string) error {
	refusal, err := keyWordRefusal(ctx, tx, key)
	if err != nil || refusal != nil {
		return firstError(err, refusal)
	}
	var taken bool
	// Unscoped, as the uniqueness index it answers for is: a key a hidden
	// project holds is still a key this one cannot have.
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM project WHERE lower(key) = lower($1) AND archived_at IS NULL)`,
		key).Scan(&taken); err != nil {
		return fmt.Errorf("check whether the key %s is taken: %w", key, err)
	}
	if taken {
		return keyTaken(key)
	}
	return nil
}

// keyWordRefusal answers why a key would misfile mail, or nil: a word senders
// bracket in subjects, or a word of a colleague's name. Agent seats are left
// out because nobody writes an agent's name in a subject.
func keyWordRefusal(ctx context.Context, tx pgx.Tx, key string) (*ProjectKeyRefusedError, error) {
	if subjectWords[strings.ToLower(key)] {
		return &ProjectKeyRefusedError{
			Code:    "key_is_common_word",
			Message: fmt.Sprintf("%s is a word often put in brackets in subject lines, so mail that is not about this project would be filed under it; choose another key", key),
		}, nil
	}
	var named bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (
		   SELECT 1 FROM app_user
		    WHERE NOT is_agent
		      AND (lower($1) = ANY (regexp_split_to_array(lower(f_unaccent(display_name)), '[^a-z0-9]+'))
		           OR lower($1) = lower(f_unaccent(greeting_name))))`,
		key).Scan(&named); err != nil {
		return nil, fmt.Errorf("check the key %s against colleagues' names: %w", key, err)
	}
	if named {
		return &ProjectKeyRefusedError{
			Code:    "key_is_colleague_name",
			Message: fmt.Sprintf("%s is a colleague's name, so mail that only mentions them would be filed under this project; choose another key", key),
		}, nil
	}
	return nil, nil
}

func firstError(err error, refusal *ProjectKeyRefusedError) error {
	if err != nil {
		return err
	}
	return refusal
}

// PreviewProjectKey answers the key a project of this name would get now.
// It runs the mint the create runs, so the form never computes a key itself.
// A create that lands before the save can still move the number on.
func (s *Store) PreviewProjectKey(ctx context.Context, name string) (string, error) {
	if err := auth.Require(ctx, projectObject, principal.ActionCreate); err != nil {
		return "", err
	}
	var key string
	err := s.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		key, err = mintProjectKey(ctx, tx, name, nil)
		return err
	})
	return key, err
}

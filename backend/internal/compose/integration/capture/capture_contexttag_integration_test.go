// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// Choosing the word a connector files under, through the real registry.
//
// The vocabulary check is the subject. It runs HERE, at the moment the word is
// chosen, and never when a record lands — a capture must not fail over a word
// somebody archived afterwards, and a mailbox going unread is a far worse
// answer to a vocabulary edit than a connector going quiet.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// putWord adds one word to the vocabulary, live or already retired.
func putWord(t *testing.T, env captureEnv, name string, archived bool) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	err := database.WithWorkspaceTx(env.e.Admin(), env.e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO tag (id, name, archived_at)
			VALUES ($1, $2, CASE WHEN $3 THEN now() ELSE NULL END)`, id, name, archived)
		return err
	})
	if err != nil {
		t.Fatalf("putting %q in the vocabulary: %v", name, err)
	}
	return id
}

func TestAConnectorTakesAWordTheVocabularyCarries(t *testing.T) {
	env := newCaptureEnv(t)
	word := putWord(t, env, "Nord inbox", false)

	view, err := env.registry.SetContextTag(seatContext(env.e, env.e.Rep1), "gmail", &word)
	if err != nil {
		t.Fatalf("filing the mailbox under a word: %v", err)
	}
	if view.ContextTag == nil {
		t.Fatal("the connection reports no word after one was chosen")
	}
	if view.ContextTag.ID != word || view.ContextTag.Name != "Nord inbox" {
		t.Errorf("the connection reports %v, want the word that was chosen", view.ContextTag)
	}
	if view.ContextTag.Archived {
		t.Error("a live word is reported as archived")
	}

	// Cleared, and the connection then files nothing — the same state as never
	// having chosen.
	cleared, err := env.registry.SetContextTag(seatContext(env.e, env.e.Rep1), "gmail", nil)
	if err != nil {
		t.Fatalf("clearing the word: %v", err)
	}
	if cleared.ContextTag != nil {
		t.Errorf("the connection still reports %v after the word was cleared", cleared.ContextTag)
	}
}

// A word the vocabulary does not carry is refused, and so is one it has
// retired. Both come back as the same answer on purpose: to the caller they
// are one fact — the word they named is not one this connector can file under
// — and telling them apart would say which archived words exist.
func TestAConnectorRefusesAWordTheVocabularyDoesNotOffer(t *testing.T) {
	for _, tc := range []struct {
		name string
		word func(t *testing.T, env captureEnv) ids.UUID
	}{
		{"never in the vocabulary", func(*testing.T, captureEnv) ids.UUID { return ids.NewV7() }},
		{"archived", func(t *testing.T, env captureEnv) ids.UUID { return putWord(t, env, "Retired inbox", true) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newCaptureEnv(t)
			word := tc.word(t, env)

			_, err := env.registry.SetContextTag(seatContext(env.e, env.e.Rep1), "gmail", &word)
			var unknown *capturemod.UnknownContextTagError
			if !errors.As(err, &unknown) {
				t.Fatalf("setting %s word: err = %v, want an unknown-word refusal", tc.name, err)
			}
			// 422 on the wire, which is what a caller naming a word that is not
			// there is owed — not a 500, and not a silent success.
			if !errors.Is(err, apperrors.ErrInvalidArgument) {
				t.Errorf("the refusal does not read as invalid argument, so the wire answers the wrong status")
			}
		})
	}
}

// A word archived AFTER it was chosen leaves the connection saying so, rather
// than going quiet with nothing to see. That is the whole reason the flag is on
// the wire: an operator whose filing stopped needs to know why.
func TestAWordArchivedAfterwardsIsReportedOnTheConnection(t *testing.T) {
	env := newCaptureEnv(t)
	word := putWord(t, env, "Nord inbox", false)
	if _, err := env.registry.SetContextTag(seatContext(env.e, env.e.Rep1), "gmail", &word); err != nil {
		t.Fatalf("filing the mailbox under a word: %v", err)
	}

	err := database.WithWorkspaceTx(env.e.Admin(), env.e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `UPDATE tag SET archived_at = now() WHERE id = $1`, word)
		return err
	})
	if err != nil {
		t.Fatalf("retiring the word: %v", err)
	}

	views, err := env.registry.Connections(seatContext(env.e, env.e.Rep1))
	if err != nil {
		t.Fatalf("reading the connections: %v", err)
	}
	var gmail *capturemod.ContextTag
	for _, v := range views {
		if v.Provider == "gmail" {
			gmail = v.ContextTag
		}
	}
	if gmail == nil {
		t.Fatal("the connection reports no word at all — a retired word is still the one that was chosen")
	}
	if !gmail.Archived {
		t.Error("the connection reports a retired word as live, so an operator whose filing stopped has " +
			"nothing to look at")
	}
}

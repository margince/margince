// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The whoami seam: the agents module asks who a call acts for, and identity
// answers. Declared as a function in agents and implemented here for the same
// reason every cross-module edge is (ADR-0054) — agents never imports a
// sibling module.

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gradionhq/margince/backend/internal/compose/promptlang"
	"github.com/gradionhq/margince/backend/internal/modules/agents"
	"github.com/gradionhq/margince/backend/internal/modules/identity"
)

// actingIdentity reads the human behind the call.
//
// It carries no object gate, and that is identity's own posture rather than an
// omission here: the question is "who is the caller", which a caller may always
// know. What it cannot do is answer for anybody else — the principal decides
// the subject, never an argument.
//
// The prose language is resolved HERE rather than in identity, because it is a
// composition of two settings that belong to different owners: the member's
// own choice and the installation's base language. identity keeps answering
// the honest empty for a person who never chose, and this seam turns that into
// the answer the agent can act on.
func actingIdentity(pool *pgxpool.Pool) agents.IdentityReader {
	service := identity.NewService(pool)
	return func(ctx context.Context) (agents.ActingIdentity, error) {
		profile, err := service.ActorProfile(ctx)
		if err != nil {
			return agents.ActingIdentity{}, err
		}
		return agents.ActingIdentity{
			UserID:        profile.UserID,
			DisplayName:   profile.DisplayName,
			Email:         profile.Email,
			Locale:        profile.Locale,
			ProseLanguage: proseLanguage(ctx, pool, profile.Locale),
			Timezone:      profile.Timezone,
		}, nil
	}
}

// proseLanguage names the language stored prose must be written in: the
// member's own choice, or the installation's base language when they never
// made one.
//
// It answers a language name rather than a code ("German", not "de"), for the
// reason promptlang.Rule already gives: the reader is a model writing a
// sentence, and a two-letter code is a lookup it can get wrong.
func proseLanguage(ctx context.Context, pool *pgxpool.Pool, locale string) string {
	if locale == "" {
		locale = identity.BaseLanguageForPrompt(ctx, pool)
	}
	return promptlang.LanguageName(locale)
}

// colleagueLister reads the workspace roster.
//
// Same posture as actingIdentity above and as identity.SeatNames: a seat is
// not a record, so there is no object to grant on, and what it discloses — a
// colleague's name and work address — who_knows and account_coverage already
// answer to any authenticated reader.
func colleagueLister(pool *pgxpool.Pool) agents.ColleagueLister {
	service := identity.NewService(pool)
	return func(ctx context.Context, q string) ([]agents.Colleague, bool, error) {
		seats, truncated, err := service.Colleagues(ctx, q)
		if err != nil {
			return nil, false, err
		}
		out := make([]agents.Colleague, 0, len(seats))
		for _, s := range seats {
			out = append(out, agents.Colleague{
				UserID: s.UserID, DisplayName: s.DisplayName, Email: s.Email,
				SeatType: s.SeatType, IsAgent: s.IsAgent,
			})
		}
		return out, truncated, nil
	}
}

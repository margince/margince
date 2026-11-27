// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/settings"
)

const (
	// TeamRulesKey is the settings key the workspace's team rules are stored under.
	TeamRulesKey    = "ai.team_rules"
	teamRulesObject = "ai_team_rules"

	// MaxTeamRulesRunes is the bound a voice profile's own rules already carry.
	MaxTeamRulesRunes = 20000
)

// TeamRules is the one text every AI draft in the workspace follows, ranked
// above the sender's own rules. Empty until an admin writes it.
//
// MachineryApplied because a draft must follow the rules whoever asked for it.
// An agent or an automation drafting under the system principal holds no
// ai_team_rules grant. Rules that reached only seats able to read them would
// miss the drafts nobody watches.
var TeamRules = settings.Define[string](TeamRulesKey, teamRulesObject, settingsAuditVerb, "", validateTeamRules).MachineryApplied()

func validateTeamRules(text string) error {
	if utf8.RuneCountInString(text) > MaxTeamRulesRunes {
		return fmt.Errorf("team rules must not exceed %d characters", MaxTeamRulesRunes)
	}
	return nil
}

// TeamRulesOf is the drafters' read: the saved text, trimmed, without the
// caller's read grant. Never use it to answer a caller; ReadTeamRules does that
// behind the gate.
func TeamRulesOf(ctx context.Context, db *database.DB) (string, error) {
	var text string
	err := db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		text, err = settings.ApplyTx(ctx, tx, TeamRules)
		return err
	})
	return strings.TrimSpace(text), err
}

// TeamRulesReader is the drafters' ungated read as a method. The voice store
// embeds it, so every surface that loads the sender's voice loads the rules
// through the same handle.
type TeamRulesReader struct{ db *database.DB }

// TeamRules returns the saved text, trimmed.
func (r TeamRulesReader) TeamRules(ctx context.Context) (string, error) {
	return TeamRulesOf(ctx, r.db)
}

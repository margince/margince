// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// The one writer of voice_profile_version and voice_profile_delta.
//
// Three paths produce a version — a build, a rollback, and the manual
// activation of a derived artifact — and each hand-wrote its own twenty-column
// INSERT. Nothing checked that the three column lists agreed, so a new column
// on the table could be added to two of them and the package would still
// compile. That is not hypothetical: one of the three left review_reasons out
// of its list entirely, and because the column is NOT NULL DEFAULT '{}' the
// omission was silent — a manually activated version read back
// indistinguishable from one with genuinely no review reasons.
//
// A struct with a field per column turns that into a compile-time question:
// a column added to the table is a field nobody fills, and a path that means
// "no reasons" has to SAY so. TestVoiceVersionsHaveOneWriter
// (backend/onevoiceversionwriter_test.go) holds the uniqueness.

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/platform/database/storekit"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// voiceVersionRow is one immutable version as its writer takes it: every
// column a caller chooses, named, with nothing left to a database default.
type voiceVersionRow struct {
	profileID      ids.UUID
	profileVersion int
	status         string
	voiceProfileMD string
	profileJSON    any
	statsJSON      any
	sourceHash     string
	sourceCount    int
	reason         string
	// predecessorVersion is the version this one supersedes; the first version
	// of a profile has none.
	predecessorVersion      *int
	modelProvider           string
	modelName               string
	builderVersion          string
	activationPolicyVersion string
	evaluation              any
	// reviewReasons is why a human must look before this version goes live.
	// Empty means "nothing to review" and is written as such rather than left
	// to the column default, so the two cases stay distinguishable.
	reviewReasons []string
	// activatedAt is when this version became the live one; nil while it is
	// still a candidate.
	activatedAt *time.Time
	source      string
	capturedBy  string
	now         time.Time
}

// voiceVersionWriteColumns is the column list this writer fills, in the order
// voiceVersionArgs builds its arguments. Declared once so a statement's
// columns, its placeholders and its bind list cannot disagree — nothing else
// in this tree checks that they do, and the three hand-written INSERTs this
// replaced each carried its own count.
//
// Held by: TestTheVoiceVersionWriterBindsEveryColumnItNames (internal/modules/ai/voice_version_write_test.go)
//
// are written. Two of the twenty happen to spell the same word as a payload key
// declared elsewhere in the package; naming only those two would make the list
// read as if they were special, which is the opposite of what it is for.
//
//nolint:goconst // the entries are column NAMES, and this is the one place they
var voiceVersionWriteColumns = []string{
	"voice_profile_id", "profile_version", "status", "voice_profile_md",
	"profile_json", "stats_json", "source_hash", "source_count", "reason",
	"predecessor_version", "model_provider", "model_name", "builder_version",
	"activation_policy_version", "evaluation_json", "review_reasons",
	"activated_at", "source", "captured_by", "updated_at",
}

// voiceVersionArgs binds one row in voiceVersionWriteColumns' order.
func voiceVersionArgs(row voiceVersionRow) []any {
	reasons := row.reviewReasons
	if reasons == nil {
		// A path that means "nothing to review" still SAYS so. Left to the
		// column's own default, it would be indistinguishable from a writer
		// that forgot the column, which is how the gap this writer exists to
		// close went unnoticed.
		reasons = []string{}
	}
	return []any{
		row.profileID, row.profileVersion, row.status, row.voiceProfileMD,
		row.profileJSON, row.statsJSON, row.sourceHash, row.sourceCount, row.reason,
		row.predecessorVersion, row.modelProvider, row.modelName, row.builderVersion,
		row.activationPolicyVersion, row.evaluation, reasons,
		row.activatedAt, row.source, row.capturedBy, row.now,
	}
}

// insertVoiceVersion writes one version and reads it back through the same
// column list every other reader of the table uses.
func insertVoiceVersion(ctx context.Context, tx pgx.Tx, row voiceVersionRow) (VoiceProfileVersion, error) {
	args := voiceVersionArgs(row)
	return scanVoiceVersion(tx.QueryRow(ctx, storekit.SQLf(
		`INSERT INTO voice_profile_version (%s) VALUES (%s) RETURNING %s`,
		strings.Join(voiceVersionWriteColumns, ", "), bindPlaceholders(len(args)), voiceVersionColumns),
		args...))
}

// voiceDeltaRow is the "what changed" timeline entry beside a version.
type voiceDeltaRow struct {
	profileID         ids.UUID
	fromVersion       *int
	toVersion         int
	classification    string
	activationOutcome string
	delta             map[string]any
}

// voiceDeltaWriteColumns is the delta row's column list, in the order
// insertVoiceDelta binds it.
var voiceDeltaWriteColumns = []string{
	"voice_profile_id", "from_version", "to_version", "classification",
	"activation_outcome", "delta_json",
}

// insertVoiceDelta writes one delta row.
func insertVoiceDelta(ctx context.Context, tx pgx.Tx, row voiceDeltaRow) error {
	args := []any{
		row.profileID, row.fromVersion, row.toVersion, row.classification,
		row.activationOutcome, storekit.JSONArg(row.delta),
	}
	_, err := tx.Exec(ctx, storekit.SQLf(`INSERT INTO voice_profile_delta (%s) VALUES (%s)`,
		strings.Join(voiceDeltaWriteColumns, ", "), bindPlaceholders(len(args))), args...)
	return err
}

// bindPlaceholders renders $1..$n for a statement whose arguments are already
// assembled, so the placeholder count is the argument count by construction
// rather than by a author counting them twice and agreeing with themselves.
func bindPlaceholders(n int) string {
	holders := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		holders = append(holders, "$"+strconv.Itoa(i))
	}
	return strings.Join(holders, ", ")
}

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
// (backend/gates/onevoiceversionwriter_test.go) holds the uniqueness.

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

// boundColumn is one column and the value going into it, together.
//
// Together is the point. Two parallel slices — a column list and an argument
// list — agree only because an author counted them twice and agreed with
// themselves, and the failure that shape allows is invisible: swap two entries
// of the same Go type and every count still matches, every gate still passes,
// and the two strings land in each other's columns. A pair cannot be swapped
// without moving the column name with it.
type boundColumn struct {
	name  string
	value any
}

// voiceVersionBindings is every column this writer fills, with the value bound
// to it. It is the single source the statement's column list, its placeholders
// and its bind list are all derived from.
//
// Held by: TestTheVoiceVersionWriterBindsEveryColumnItNames (internal/modules/ai/voice_version_write_test.go)
//
//nolint:goconst // the entries are column NAMES, and this is the one place they are written. Two of the twenty happen to spell the same word as a payload key declared elsewhere in the package; naming only those two would make the list read as if they were special, which is the opposite of what it is for.
func voiceVersionBindings(row voiceVersionRow) []boundColumn {
	reasons := row.reviewReasons
	if reasons == nil {
		// A path that means "nothing to review" still SAYS so. Left to the
		// column's own default, it would be indistinguishable from a writer
		// that forgot the column, which is how the gap this writer exists to
		// close went unnoticed.
		reasons = []string{}
	}
	return []boundColumn{
		{"voice_profile_id", row.profileID},
		{"profile_version", row.profileVersion},
		{"status", row.status},
		{"voice_profile_md", row.voiceProfileMD},
		{"profile_json", row.profileJSON},
		{"stats_json", row.statsJSON},
		{"source_hash", row.sourceHash},
		{"source_count", row.sourceCount},
		{"reason", row.reason},
		{"predecessor_version", row.predecessorVersion},
		{"model_provider", row.modelProvider},
		{"model_name", row.modelName},
		{"builder_version", row.builderVersion},
		{"activation_policy_version", row.activationPolicyVersion},
		{"evaluation_json", row.evaluation},
		{"review_reasons", reasons},
		{"activated_at", row.activatedAt},
		{"source", row.source},
		{"captured_by", row.capturedBy},
		{"updated_at", row.now},
	}
}

// voiceVersionWriteColumns is the column list, derived from the bindings so it
// cannot fall out of step with them. The row is a zero value because only the
// NAMES are read.
func voiceVersionWriteColumns() []string {
	return namesOf(voiceVersionBindings(voiceVersionRow{}))
}

// namesOf and valuesOf split a binding list into the two halves a statement
// needs, at the moment it needs them and never before.
func namesOf(bound []boundColumn) []string {
	names := make([]string, 0, len(bound))
	for _, column := range bound {
		names = append(names, column.name)
	}
	return names
}

func valuesOf(bound []boundColumn) []any {
	values := make([]any, 0, len(bound))
	for _, column := range bound {
		values = append(values, column.value)
	}
	return values
}

// insertVoiceVersion writes one version and reads it back through the same
// column list every other reader of the table uses.
func insertVoiceVersion(ctx context.Context, tx pgx.Tx, row voiceVersionRow) (VoiceProfileVersion, error) {
	bound := voiceVersionBindings(row)
	args := valuesOf(bound)
	return scanVoiceVersion(tx.QueryRow(ctx, storekit.SQLf(
		`INSERT INTO voice_profile_version (%s) VALUES (%s) RETURNING %s`,
		strings.Join(namesOf(bound), ", "), bindPlaceholders(len(args)), voiceVersionColumns),
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

// voiceDeltaBindings is the delta row's columns with their values, paired for
// the same reason the version's are.
func voiceDeltaBindings(row voiceDeltaRow) []boundColumn {
	return []boundColumn{
		{"voice_profile_id", row.profileID},
		{"from_version", row.fromVersion},
		{"to_version", row.toVersion},
		{"classification", row.classification},
		{"activation_outcome", row.activationOutcome},
		{"delta_json", storekit.JSONArg(row.delta)},
	}
}

// voiceDeltaWriteColumns is the delta's column list, derived from its bindings.
func voiceDeltaWriteColumns() []string {
	return namesOf(voiceDeltaBindings(voiceDeltaRow{}))
}

// insertVoiceDelta writes one delta row.
func insertVoiceDelta(ctx context.Context, tx pgx.Tx, row voiceDeltaRow) error {
	bound := voiceDeltaBindings(row)
	args := valuesOf(bound)
	_, err := tx.Exec(ctx, storekit.SQLf(`INSERT INTO voice_profile_delta (%s) VALUES (%s)`,
		strings.Join(namesOf(bound), ", "), bindPlaceholders(len(args))), args...)
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

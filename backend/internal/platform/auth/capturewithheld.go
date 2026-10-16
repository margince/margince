// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
)

// Whether capture privacy actually withheld a row from this caller.
//
// Row scope is a question about the principal, so an answer's shape can be
// described before the read. Capture privacy cannot be. It binds every human on
// contact and company. So "the arm was rendered" is true of nearly every
// answer, and says as little as a warning on none. What a reader needs is
// whether a row was withheld, and only the read can say.

// WithheldReport is written by a read path and read by whoever describes the
// answer. A caller that installs none pays a map lookup per list and nothing
// else, which is what keeps this off the interactive path.
type WithheldReport struct{ withheld atomic.Bool }

// Withheld reports whether any read under this context lost a row to capture
// privacy. It never says how many: a count is the side channel the boundary
// exists to close.
func (r *WithheldReport) Withheld() bool { return r != nil && r.withheld.Load() }

type withheldReportKey struct{}

// WithWithheldReport installs a report for the reads made under the returned
// context. The caller holds the only handle, so one request's answer cannot
// read another's.
func WithWithheldReport(ctx context.Context) (context.Context, *WithheldReport) {
	report := &WithheldReport{}
	return context.WithValue(ctx, withheldReportKey{}, report), report
}

func withheldReportFrom(ctx context.Context) *WithheldReport {
	report, _ := ctx.Value(withheldReportKey{}).(*WithheldReport)
	return report
}

// NoteCaptureWithheld records whether a row this caller's scope would admit is
// withheld by capture privacy alone.
//
// The two predicates come from the same builder the read itself used, so a
// change to how either arm composes moves this probe with it. Spelled out here
// instead, the two would drift. This would then answer about a boundary the
// read no longer enforces.
//
// It runs only for a caller holding a report, and only until the first row is
// found. The answer is a yes or a no, so a second yes adds nothing.
func NoteCaptureWithheld(ctx context.Context, tx pgx.Tx, table string) error {
	report := withheldReportFrom(ctx)
	if report == nil || report.withheld.Load() || !ownerPrivateTables[table] {
		return nil
	}
	p, err := rbacActor(ctx)
	if err != nil {
		return err
	}
	if actsForTheInstallation(p) {
		return nil
	}
	args := []any{}
	arg := func(v any) int { args = append(args, v); return len(args) }
	visible := predicateFor(p, table, arg, withCapturePrivacy, asClassified, anyShare)("")
	ifShared := predicateFor(p, table, arg, withoutCapturePrivacy, asClassified, anyShare)("")

	var withheld bool
	row := tx.QueryRow(ctx, fmt.Sprintf(
		`SELECT EXISTS (SELECT 1 FROM %s
		    WHERE archived_at IS NULL AND NOT (%s) AND (%s))`,
		pgx.Identifier{table}.Sanitize(), visible, ifShared), args...)
	if err := row.Scan(&withheld); err != nil {
		return fmt.Errorf("auth: probing what capture privacy withheld: %w", err)
	}
	if withheld {
		report.withheld.Store(true)
	}
	return nil
}

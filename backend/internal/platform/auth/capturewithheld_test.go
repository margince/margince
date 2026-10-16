// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The guards are the reason an interactive read can call this on every page.
// Each one returns before the transaction is touched, so a nil tx here is the
// assertion: reaching the probe would panic.
func TestTheProbeIsSkippedWhenNobodyAskedForTheAnswer(t *testing.T) {
	t.Parallel()
	ctx := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, UserID: ids.NewV7(),
	})

	if err := NoteCaptureWithheld(ctx, nil, "contact"); err != nil {
		t.Errorf("a read with no report asked the database anyway: %v", err)
	}
}

// Capture privacy lives on two tables. Probing any other one would spend a
// query to learn what the table's shape already says.
func TestTheProbeIsSkippedOnATableWithoutCapturePrivacy(t *testing.T) {
	t.Parallel()
	ctx, report := WithWithheldReport(principal.WithActor(context.Background(),
		principal.Principal{Type: principal.PrincipalHuman, UserID: ids.NewV7()}))

	if err := NoteCaptureWithheld(ctx, nil, "deal"); err != nil {
		t.Errorf("a table without capture privacy was probed: %v", err)
	}
	if report.Withheld() {
		t.Error("a table without capture privacy reported withholding")
	}
}

// A run with no human behind it reads the capture-private arm away. Nothing is
// left for the probe to find, so the query is not worth spending.
func TestTheProbeIsSkippedForARunActingForTheInstallation(t *testing.T) {
	t.Parallel()
	ctx, report := WithWithheldReport(principal.WithActor(context.Background(),
		principal.Principal{Type: principal.PrincipalSystem}))

	if err := NoteCaptureWithheld(ctx, nil, "contact"); err != nil {
		t.Errorf("a system run was probed: %v", err)
	}
	if report.Withheld() {
		t.Error("a system run reported withholding, which its own predicate rules out")
	}
}

// Withheld is read by whoever describes the answer, including before any read
// has run. A nil report is the ordinary case on the interactive path.
func TestAnAnswerNobodyReportedOnSaysNothingWasWithheld(t *testing.T) {
	t.Parallel()
	var absent *WithheldReport
	if absent.Withheld() {
		t.Error("a report nobody installed claims a row was withheld")
	}
	_, report := WithWithheldReport(context.Background())
	if report.Withheld() {
		t.Error("a fresh report claims a row was withheld before any read")
	}
}

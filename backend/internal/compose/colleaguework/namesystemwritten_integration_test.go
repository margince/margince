// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package colleaguework

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// A meeting Margince wrote is named in the report's language on the colleague
// work figure, as every other word of that report is.
func TestAColleagueWorkFactMarginceWroteIsNamedInTheReportsLanguage(t *testing.T) {
	e := integration.Setup(t)
	subject := systemtext.Literal("automation.new_lead")
	meeting, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, Source: "manual",
		SystemText: &systemtext.Message{Subject: systemtext.Sentence("automation.new_lead")},
	})
	if err != nil {
		t.Fatal(err)
	}
	facts := []reporting.Fact{{SourceID: ids.UUID(meeting.Id)}, {SourceID: ids.NewV7()}}
	facts[0].Row.Label, facts[1].Row.Label = subject, "Kickoff"
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return NameSystemWritten(e.Admin(), tx, facts, textlang.German)
	}); err != nil {
		t.Fatal(err)
	}
	if facts[0].Row.Label != "Beim neuen Lead nachfassen" || facts[1].Row.Label != "Kickoff" {
		t.Errorf("the facts read %q and %q", facts[0].Row.Label, facts[1].Row.Label)
	}
}

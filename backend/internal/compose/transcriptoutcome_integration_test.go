// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// reportFor answers the poll for one finished reading as the reader in ctx.
func (e *transcriptEnv) reportFor(t *testing.T, ctx context.Context, read activities.TranscriptRead) crmcontracts.TranscriptReadReport {
	t.Helper()
	engine := &transcriptReadEngine{activities: e.Activities, seats: identity.NewService(e.Pool), pool: e.Pool}
	rec := httptest.NewRecorder()
	engine.report(rec, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx),
		openapi_types.UUID(e.activity.UUID), openapi_types.UUID(read.ID))
	if rec.Code != http.StatusOK {
		t.Fatalf("the poll answered %d: %s", rec.Code, rec.Body)
	}
	var report crmcontracts.TranscriptReadReport
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	return report
}

func detailOf(report crmcontracts.TranscriptReadReport) string {
	if report.Detail == nil {
		return ""
	}
	return *report.Detail
}

// speaking is ctx with the reader's chosen display language.
func speaking(ctx context.Context, locale string) context.Context {
	actor, _ := principal.Actor(ctx)
	actor.Locale = locale
	return principal.WithActor(ctx, actor)
}

// A task the reading gave a colleague is "for NAME" to the one who logged the
// meeting, and "for you" to the colleague. The stored outcome names the seat.
func TestTheReportSaysWhoATranscriptTaskWentTo(t *testing.T) {
	e := setupTranscript(t)
	e.WsExec(t, `UPDATE app_user SET display_name = 'Priya Raman' WHERE id = $1`, e.Rep2)
	read := e.read(t, cannedBrain{reply: attributedReply(t, "Priya Raman", "seat:"+e.Rep2.String(), 0.9)})
	if read.Outcome == nil || len(read.Outcome.Tasks) != 1 || read.Outcome.Tasks[0].SeatID != e.Rep2 {
		t.Fatalf("the stored outcome is %+v, want one task for the colleague", read.Outcome)
	}
	if got := detailOf(e.reportFor(t, e.ctx, read)); got != "added 1 task for Priya Raman" {
		t.Errorf("the logger reads %q", got)
	}
	priya := e.As(e.Rep2, []ids.UUID{e.Team1}, transcriptPerms)
	if got := detailOf(e.reportFor(t, priya, read)); got != "added 1 task for you" {
		t.Errorf("the colleague reads %q", got)
	}
}

// A proposal put to the colleague who made the promise is "sent to NAME to
// confirm" for the logger, with no status for them to read, and theirs to
// decide for the colleague. A German reader gets German on an English
// installation.
func TestTheReportSaysWhoATranscriptProposalWaitsOn(t *testing.T) {
	e := setupTranscript(t)
	e.WsExec(t, `UPDATE app_user SET display_name = 'Priya Raman' WHERE id = $1`, e.Rep2)
	read := e.read(t, cannedBrain{reply: attributedReply(t, "Priya Raman", "seat:"+e.Rep2.String(), 0.75)})
	if len(read.ProposalIDs) != 1 {
		t.Fatalf("want the unsure promise proposed, got %d proposals", len(read.ProposalIDs))
	}

	logger := e.reportFor(t, e.ctx, read)
	if got := detailOf(logger); got != "sent 1 suggestion to Priya Raman to confirm" {
		t.Errorf("the logger reads %q", got)
	}
	if len(logger.Proposals) != 1 || logger.Proposals[0].AskedOf.IsYou ||
		logger.Proposals[0].AskedOf.Name == nil || *logger.Proposals[0].AskedOf.Name != "Priya Raman" ||
		ids.UUID(logger.Proposals[0].Id) != read.ProposalIDs[0] {
		t.Errorf("the logger's proposals are %+v, want the one approval asked of Priya Raman", logger.Proposals)
	}

	priya := e.reportFor(t, e.As(e.Rep2, []ids.UUID{e.Team1}, transcriptPerms), read)
	if got := detailOf(priya); got != "you were asked to confirm 1 suggestion" {
		t.Errorf("the colleague reads %q", got)
	}
	if len(priya.Proposals) != 1 || !priya.Proposals[0].AskedOf.IsYou {
		t.Errorf("the colleague's proposals are %+v, want their own", priya.Proposals)
	}

	german := detailOf(e.reportFor(t, speaking(e.ctx, "de"), read))
	if german != "1 Vorschlag an Priya Raman zur Bestätigung gesendet" {
		t.Errorf("a German reader reads %q", german)
	}
}

// A reading finished before the outcome was kept as facts still answers the
// sentence it stored, and phrases nothing for the reader.
func TestAReadingFromBeforeFactsKeepsItsOwnSentence(t *testing.T) {
	e := setupTranscript(t)
	read := e.read(t, cannedBrain{reply: `{"proposals":[]}`})
	e.WsExec(t, `UPDATE transcript_read SET outcome = NULL, status_detail = 'an old sentence' WHERE id = $1`, read.ID)
	report := e.reportFor(t, e.ctx, read)
	if report.Detail != nil || report.StatusDetail == nil || *report.StatusDetail != "an old sentence" || report.Proposals == nil {
		t.Errorf("an old reading answered detail=%v status_detail=%v proposals=%v; want its stored sentence alone and an empty list",
			report.Detail, report.StatusDetail, report.Proposals)
	}
}

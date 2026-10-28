// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A sentence the server words itself is stored once, in the installation's
// language, and read by everyone who can see the account. Each reader gets it
// in the language they chose.

import (
	"context"
	"maps"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestATechnicalChangeReadsInEachReadersLanguage(t *testing.T) {
	e := integration.Setup(t)
	company := e.SeedCompany(t, "Stack Co", nil)
	startsUsingNginx(t, e, company)

	if stored := e.WsScalar(t, `SELECT summary FROM signal WHERE entity_id = $1 AND kind = $2`,
		company, kindTechnicalChange); stored != "Now uses nginx" {
		t.Fatalf("an English installation stored %q, want the English sentence", stored)
	}
	for lang, want := range map[textlang.Lang]string{
		textlang.English:    "Now uses nginx",
		textlang.German:     "Setzt jetzt nginx ein",
		textlang.Vietnamese: "Hiện đang dùng nginx",
	} {
		if got := readSummary(t, e, readerIn(t, e, signalSeat(), lang), company, kindTechnicalChange); got != want {
			t.Errorf("a %s reader reads %q, want %q", lang, got, want)
		}
	}
}

func TestTheAccountStripStatesTheWorstSignalInTheReadersLanguage(t *testing.T) {
	e := integration.Setup(t)
	company := e.SeedCompany(t, "Strip Co", nil)
	startsUsingNginx(t, e, company)

	if got := stripSummary(t, e, readerIn(t, e, signalSeat(), textlang.Vietnamese), company); got != "Hiện đang dùng nginx" {
		t.Errorf("the strip states %q to a Vietnamese reader, want the Vietnamese sentence", got)
	}
}

// A reader who chose no language gets the stored sentence, in the
// installation's language, even on a seat that cannot read that setting.
func TestAReaderWhoChoseNoLanguageReadsTheInstallationsLanguage(t *testing.T) {
	e := integration.Setup(t)
	integration.SetBaseLanguage(e.Admin(), t, e.Pool, "de")
	company := e.SeedCompany(t, "Stapel GmbH", nil)
	startsUsingNginx(t, e, company)

	narrow := readerIn(t, e, principal.Permissions{
		Objects: map[string]principal.ObjectGrant{
			"company": {Read: true}, "signal": {Read: true},
		},
		RowScope: principal.RowScopeAll,
	}, "")
	if got := readSummary(t, e, narrow, company, kindTechnicalChange); got != "Setzt jetzt nginx ein" {
		t.Errorf("the list reads %q, want the stored German sentence", got)
	}
	if got := stripSummary(t, e, readerIn(t, e, signalSeat(), ""), company); got != "Setzt jetzt nginx ein" {
		t.Errorf("the strip states %q, want the stored German sentence", got)
	}
}

// The intro draft goes to a contact outside the company, so its language
// follows the stored summary, never the rep's display language.
func TestAnIntroDraftKeepsTheStoredSummaryForAReaderInAnotherLanguage(t *testing.T) {
	e := integration.Setup(t)
	company := e.SeedCompany(t, "Route Co", nil)
	employedAt(t, e, company, "Rae Route")
	startsUsingNginx(t, e, company)
	reader := readerIn(t, e, signalSeat(), textlang.Vietnamese)
	store := signals.NewStore(e.DB(), signalStrength{contacts: e.Contacts}).WithSummaryWords(signalSummaryWords)
	kind := kindTechnicalChange
	listed, _, err := store.ListSignals(reader, signals.ListSignalsInput{CompanyID: &company, Kind: &kind})
	if err != nil || len(listed) != 1 {
		t.Fatalf("listing the account's signal: %v, %d rows", err, len(listed))
	}

	path, err := store.IntroPath(reader, ids.From[ids.SignalKind](ids.UUID(listed[0].Id)), time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("the intro path: %v", err)
	}
	if !strings.Contains(path.NextMove.DraftBody, "Now uses nginx") {
		t.Errorf("the draft does not carry the stored English summary: %q", path.NextMove.DraftBody)
	}
	if !strings.HasPrefix(path.NextMove.DraftSubject, "Getting in touch about") {
		t.Errorf("the draft subject %q is not the English one", path.NextMove.DraftSubject)
	}
}

func TestAGhostedThreadReadsInTheReadersLanguage(t *testing.T) {
	e := integration.Setup(t)
	wrote := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	company := e.SeedCompany(t, "Silent Co", nil)
	e.WsExec(t, `UPDATE company SET lifecycle = 'opportunity' WHERE id = $1`, company)
	contact := employedAt(t, e, company, "Sam Silent")
	logMail(t, e, contact, "inbound", wrote.AddDate(0, 0, -5))
	logMail(t, e, contact, "outbound", wrote)
	scan := principal.SystemActing(e.Admin(), signalScanActor)
	if err := database.WithWorkspaceTx(scan, e.Pool, func(tx pgx.Tx) error {
		_, err := WriteGhostedSignals(scan, tx, wrote.AddDate(0, 0, 15))
		return err
	}); err != nil {
		t.Fatal(err)
	}

	for lang, want := range map[textlang.Lang]string{
		textlang.German:     "Wir haben vor mindestens 14 Tagen geschrieben und niemand hat geantwortet.",
		textlang.Vietnamese: "Chúng ta đã viết cách đây ít nhất 14 ngày và chưa ai trả lời.",
	} {
		if got := readSummary(t, e, readerIn(t, e, signalSeat(), lang), company, kindGhostedThread); got != want {
			t.Errorf("a %s reader reads %q, want %q", lang, got, want)
		}
	}
}

func TestAQuietProjectReadsInTheReadersLanguage(t *testing.T) {
	e := integration.Setup(t)
	company := e.SeedCompany(t, "Quiet Co", nil)
	owner := ids.From[ids.UserKind](e.Rep1)
	created, err := e.Projects.CreateProject(e.Admin(), projects.CreateProjectInput{
		Name: "ERP rollout", CompanyID: ids.From[ids.CompanyKind](company), OwnerID: &owner, Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the project: %v", err)
	}
	project := ids.From[ids.ProjectKind](ids.UUID(created.Id))
	if _, err := e.Projects.AdvanceProjectPhase(e.Admin(), project, projects.AdvanceProjectPhaseInput{ToPhase: "pursuing"}); err != nil {
		t.Fatalf("advancing the project: %v", err)
	}
	quietSince := time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	e.WsExec(t, `UPDATE project SET created_at = $2 WHERE id = $1`, project, quietSince)
	scan := principal.SystemActing(e.Admin(), signalScanActor)
	if err := database.WithWorkspaceTx(scan, e.Pool, func(tx pgx.Tx) error {
		_, err := WriteProjectQuietSignals(scan, tx, quietSince.AddDate(0, 0, 45))
		return err
	}); err != nil {
		t.Fatal(err)
	}

	got := readSummary(t, e, readerIn(t, e, signalSeat(), textlang.Vietnamese), company, kindProjectGoneQuiet)
	if want := "Không có gì được lưu cho ERP rollout trong ít nhất 30 ngày."; got != want {
		t.Errorf("a Vietnamese reader reads %q, want %q", got, want)
	}
}

// A row that kept no parts reads as it was written, whatever its kind.
func TestASignalWithoutPartsReadsAsItWasStored(t *testing.T) {
	e := integration.Setup(t)
	company := e.SeedCompany(t, "Filed Co", nil)
	subject := "company"
	if _, err := signals.NewStore(e.DB(), nil).CreateSignal(readerIn(t, e, signalSeat(), textlang.English), signals.CreateSignalInput{
		Kind: kindTechnicalChange, EntityType: &subject, EntityID: &company,
		Summary: "Now uses nginx", Source: "manual",
	}); err != nil {
		t.Fatalf("filing the signal: %v", err)
	}

	if got := readSummary(t, e, readerIn(t, e, signalSeat(), textlang.Vietnamese), company, kindTechnicalChange); got != "Now uses nginx" {
		t.Errorf("a Vietnamese reader reads %q, want the stored sentence", got)
	}
}

// startsUsingNginx records, through the homepage lane and the production
// recorder, that the company now runs nginx.
func startsUsingNginx(t *testing.T, e *integration.Env, company ids.UUID) {
	t.Helper()
	if err := e.Contacts.ApplyTechnicalEnrichment(e.Admin(), contacts.TechnicalEnrichment{
		CompanyID: ids.From[ids.CompanyKind](company),
		Completed: []contacts.TechnicalLane{contacts.LaneHomepage},
		Observations: []contacts.TechnicalObservation{{
			Field: contacts.FactTechnology, ValueKey: "nginx", Value: "nginx",
			Evidence: "Server: nginx", SourceURL: "https://stack.example",
		}},
		ObservedAt: time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC),
	}, technicalChangeRecorder()); err != nil {
		t.Fatalf("recording the technical change: %v", err)
	}
}

// signalSeat is the admin seat, which may also read and file signals.
func signalSeat() principal.Permissions {
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["signal"] = principal.ObjectGrant{Create: true, Read: true}
	return perms
}

// readerIn is a seat reading in a chosen display language, or in none when
// lang is empty.
func readerIn(t *testing.T, e *integration.Env, perms principal.Permissions, lang textlang.Lang) context.Context {
	t.Helper()
	ctx := e.As(e.AdminUser, nil, perms)
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("the seat carries no principal to give a language")
	}
	actor.Locale = string(lang)
	return principal.WithActor(ctx, actor)
}

// readSummary reads the account's one signal of this kind through the list and
// the single read, which must agree.
func readSummary(t *testing.T, e *integration.Env, reader context.Context, company ids.UUID, kind string) string {
	t.Helper()
	store := signals.NewStore(e.DB(), nil).WithSummaryWords(signalSummaryWords)
	listed, _, err := store.ListSignals(reader, signals.ListSignalsInput{CompanyID: &company, Kind: &kind})
	if err != nil {
		t.Fatalf("listing the account's signals: %v", err)
	}
	if len(listed) != 1 {
		t.Fatalf("the account lists %d %s signals, want 1", len(listed), kind)
	}
	one, err := store.GetSignal(reader, ids.From[ids.SignalKind](ids.UUID(listed[0].Id)), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the signal: %v", err)
	}
	if one.Summary != listed[0].Summary {
		t.Errorf("the single read says %q and the list %q", one.Summary, listed[0].Summary)
	}
	return listed[0].Summary
}

func stripSummary(t *testing.T, e *integration.Env, reader context.Context, company ids.UUID) string {
	t.Helper()
	now := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	service := company360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects,
		approvals.NewService(InstallationDB(e.Pool)), func() time.Time { return now }).
		WithSignalWords(signalSummaryWords)
	page, err := service.Assemble(reader, ids.From[ids.CompanyKind](company))
	if err != nil {
		t.Fatalf("assembling the account page: %v", err)
	}
	if page.StateStrip == nil || page.StateStrip.Signal == nil {
		t.Fatal("the strip states no signal, want the technical change")
	}
	return page.StateStrip.Signal.Summary
}

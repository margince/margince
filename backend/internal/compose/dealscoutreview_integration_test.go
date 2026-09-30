// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Deal Scout's harder edges against a real database: the file-name matcher,
// the cap choosing among qualifying companies, the object grants a reader
// needs, an acceptance racing a deal opened by hand, a cited message going
// away behind a live signal, and dropping the suggested amount.

import (
	"context"
	"errors"
	"maps"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/extraction"
)

func TestAProposalIsNamedByAWordOfItsFileName(t *testing.T) {
	e := setupScout(t)
	const pdf = "application/pdf"
	cases := []struct {
		filename, contentType string
		want                  bool
	}{
		{"Angebot_2026.pdf", pdf, true},
		{"Angebot2026.pdf", pdf, true},
		{"Kostenvoranschlag Müller GmbH.pdf", pdf, true},
		{"Proposal - Acme.docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", true},
		{"SOW_v3.pdf", pdf + "; name=SOW_v3.pdf", true},
		{"statement-of-work.pdf", pdf, true},
		{"Báo giá dự án.pdf", pdf, true},
		{"Hợp đồng dịch vụ.pdf", pdf, true},
		{"hop_dong.pdf", pdf, true},
		{"Quote.pdf", "", true},

		{"Newsletter_Angebote.pdf", pdf, false},
		{"Angebot_und_Rechnung.pdf", pdf, false},
		{"Invoice-Quote-123.pdf", pdf, false},
		{"Hóa đơn báo giá.pdf", pdf, false},
		{"Vertragsnummer.pdf", pdf, false},
		{"Angebot.png", "image/png", false},
		{"Angebot.pdf", "image/png", false},
		{"Angebot.exe", "", false},
		{"Quotes.pdf", pdf, false},
		{"Agenda.pdf", pdf, false},
	}
	ctx := e.system()
	for _, c := range cases {
		args := []any{c.filename, c.contentType}
		matcher := proposalDocumentSQL("$1::text", "$2::text", func(v any) int { args = append(args, v); return len(args) })
		var got bool
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			return tx.QueryRow(ctx, `SELECT `+matcher, args...).Scan(&got)
		}); err != nil {
			t.Fatalf("matching %q: %v", c.filename, err)
		}
		if got != c.want {
			t.Errorf("%q (%q) reads as a proposal: %v, want %v", c.filename, c.contentType, got, c.want)
		}
	}
}

func TestTheCapChoosesAmongCompaniesWhoseEvidenceQualifies(t *testing.T) {
	e := setupScout(t)
	acme := e.SeedCompany(t, "Acme GmbH", nil)
	dana := e.employee(t, "Dana Buyer", acme)
	e.meeting(e.Admin(), t, "Scoping workshop", &dana, e.daysAgo(20))
	// Three newer companies, each with a lone signal: evidence, but no rule fires.
	for _, name := range []string{"Lone One", "Lone Two", "Lone Three"} {
		lone := e.SeedCompany(t, name, nil)
		e.signal(t, "new_opportunity", lone, e.email(t, "Idea", "inbound", lone, e.daysAgo(3)), e.daysAgo(2), ids.Nil)
	}

	ctx := e.system()
	var pass DealScoutPass
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		pass, err = scoutPass(ctx, tx, e.now, 2)
		return err
	}); err != nil {
		t.Fatalf("the scout pass: %v", err)
	}
	if pass.Raised != 1 || e.stored(t, acme, deals.SuggestionOpen) != 1 {
		t.Fatalf("a pass capped at two raised %d, Acme %d; want Acme's meeting suggested past three lone signals",
			pass.Raised, e.stored(t, acme, deals.SuggestionOpen))
	}
}

// withoutGrant is a rep missing one object grant.
func (e *scoutEnv) withoutGrant(user, team ids.UUID, object string) context.Context {
	perms := scoutRepPerms()
	perms.Objects = maps.Clone(perms.Objects)
	delete(perms.Objects, object)
	return e.As(user, []ids.UUID{team}, perms)
}

func TestAReaderWithoutTheGrantForAKindOfEvidenceSeesNoSuggestionCitingIt(t *testing.T) {
	e := setupScout(t)
	met := e.SeedCompany(t, "Met GmbH", nil)
	dana := e.employee(t, "Dana Buyer", met)
	e.meeting(e.Admin(), t, "Scoping workshop", &dana, e.daysAgo(3))
	signalled := e.SeedCompany(t, "Signalled GmbH", nil)
	mail := e.email(t, "Next steps", "inbound", signalled, e.daysAgo(6))
	e.signal(t, "new_opportunity", signalled, mail, e.daysAgo(5), ids.Nil)
	e.signal(t, "commitment_made", signalled, mail, e.daysAgo(2), ids.Nil)
	e.pass(t)

	full := e.rep(e.Rep1, e.Team1)
	for _, company := range []ids.UUID{met, signalled} {
		if n := len(e.suggestions(full, t, company)); n != 1 {
			t.Fatalf("a rep holding every grant sees %d suggestions, want 1", n)
		}
	}
	noActivity := e.withoutGrant(e.Rep1, e.Team1, "activity")
	if n := len(e.suggestions(noActivity, t, met)); n != 0 {
		t.Errorf("a rep without activity read sees %d suggestions citing a meeting", n)
	}
	noSignal := e.withoutGrant(e.Rep1, e.Team1, "signal")
	if n := len(e.suggestions(noSignal, t, signalled)); n != 0 {
		t.Errorf("a rep without signal read sees %d suggestions citing signals", n)
	}
	if n, err := e.Deals.CountOpenSuggestions(noSignal); err != nil || n != 1 {
		t.Errorf("a rep without signal read counts %d (err %v), want only the meeting's", n, err)
	}
}

func TestAcceptingAfterADealWasOpenedByHandOpensNoSecondDeal(t *testing.T) {
	e := setupScout(t)
	acme := e.SeedCompany(t, "Acme GmbH", nil)
	dana := e.employee(t, "Dana Buyer", acme)
	e.meeting(e.Admin(), t, "Scoping workshop", &dana, e.daysAgo(3))
	e.pass(t)
	suggestion := e.onlySuggestion(e.Admin(), t, acme)
	company := ids.From[ids.CompanyKind](acme)
	if _, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Opened by hand", PipelineID: e.pipeline, StageID: e.open, CompanyID: &company, Source: "manual",
	}); err != nil {
		t.Fatalf("opening the deal by hand: %v", err)
	}

	_, err := e.decider().AcceptSuggestion(e.Admin(), suggestion.ID, deals.AcceptSuggestionInput{})
	var superseded *deals.SuggestionSupersededError
	if !errors.As(err, &superseded) || !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("accepting = %v, want a conflict saying the company already has a deal", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM deal WHERE company_id = $1`, acme); n != 1 {
		t.Fatalf("the company holds %d deals, want only the one opened by hand", n)
	}
	if n := e.stored(t, acme, deals.SuggestionSuperseded); n != 1 {
		t.Fatalf("%d superseded suggestions, want the refused one retired", n)
	}
}

func TestAMessageCitedByALiveSignalGoingAwayRetiresTheSuggestion(t *testing.T) {
	e := setupScout(t)
	acme := e.SeedCompany(t, "Acme GmbH", nil)
	cited := e.email(t, "Next steps", "inbound", acme, e.daysAgo(6))
	e.signal(t, "new_opportunity", acme, cited, e.daysAgo(5), ids.Nil)
	e.signal(t, "commitment_made", acme, e.email(t, "Confirmed", "inbound", acme, e.daysAgo(6)), e.daysAgo(2), ids.Nil)
	e.pass(t)
	e.onlySuggestion(e.signalReader(), t, acme)

	if _, err := e.Activities.ArchiveActivity(e.Admin(), ids.From[ids.ActivityKind](cited), nil); err != nil {
		t.Fatalf("archiving the cited message: %v", err)
	}
	if pass := e.pass(t); pass.Superseded != 1 {
		t.Fatalf("the pass superseded %d suggestions, want the one whose signal cites a gone message", pass.Superseded)
	}
	if n := e.stored(t, acme, deals.SuggestionOpen); n != 0 {
		t.Fatalf("%d open suggestions still block the company", n)
	}
}

func TestAcceptingWithNoAmountDropsTheSuggestedOne(t *testing.T) {
	e := setupScout(t)
	priced := e.SeedCompany(t, "Priced GmbH", nil)
	proposal := e.document(t, e.email(t, "Angebot", "outbound", priced, e.daysAgo(2)), "Angebot.pdf", "application/pdf")
	e.reading(t, proposal,
		extraction.ExtractedField{Field: "amount_minor", Value: "1250000", Confidence: "high"},
		extraction.ExtractedField{Field: "currency", Value: "EUR", Confidence: "high"})
	e.pass(t)
	suggestion := e.onlySuggestion(e.Admin(), t, priced)
	if suggestion.AmountMinor == nil {
		t.Fatal("the suggestion carries no amount to drop")
	}

	amount := int64(5)
	if _, err := e.decider().AcceptSuggestion(e.Admin(), suggestion.ID, deals.AcceptSuggestionInput{
		NoAmount: true, AmountMinor: &amount,
	}); err == nil {
		t.Fatal("no_amount with an amount was accepted")
	}
	out, err := e.decider().AcceptSuggestion(e.Admin(), suggestion.ID, deals.AcceptSuggestionInput{NoAmount: true})
	if err != nil {
		t.Fatalf("accepting: %v", err)
	}
	deal, err := e.Deals.GetDeal(e.Admin(), ids.From[ids.DealKind](out.DealID), storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the deal: %v", err)
	}
	if deal.AmountMinor != nil || deal.Currency != nil {
		t.Fatalf("the deal opened at %v %v, want no amount", deal.AmountMinor, deal.Currency)
	}
}

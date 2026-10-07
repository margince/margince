// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package company360

// The set reader the ranked queue asks about every account on a page, held
// against the page it must agree with.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	company360svc "github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A queue row about an account and the account's own page answer "who wrote
// last" with the same two dates, and the set read leaves out what the record
// read would refuse: a colleague's private account, and an archived one.
func TestTheSetLastTouchAgreesWithTheCompanyPage(t *testing.T) {
	e := integration.Setup(t)
	owner := integration.OwnerConn(t)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)

	company := e.SeedCompany(t, "Scale Commerce", &e.Rep1)
	employee := e.SeedContact(t, "Christian Contact", &e.Rep1)
	employAt(t, e, employee, company, nil)
	inbound := integration.AccountMailDirectedAt(t, owner, e.WS, "their reply", "inbound", company360Clock.Add(-30*time.Hour))
	integration.LinkActivity(t, owner, inbound, "contact", employee)
	outbound := integration.AccountMailDirectedAt(t, owner, e.WS, "our nudge", "outbound", company360Clock.Add(-2*time.Hour))
	integration.LinkToCompany(t, e, outbound, company)

	// Accounts are readable by every seat; capture privacy is what hides one.
	private := e.SeedCompany(t, "Elsewhere AG", &e.Rep3)
	e.MakeCapturePrivate(t, "company", private, e.Rep3)
	theirs := integration.AccountMailDirectedAt(t, owner, e.WS, "their account", "inbound", company360Clock.Add(-4*time.Hour))
	integration.LinkToCompany(t, e, theirs, private)
	archived := e.SeedCompany(t, "Closed GmbH", &e.Rep1)
	retired := integration.AccountMailDirectedAt(t, owner, e.WS, "before we closed", "inbound", company360Clock.Add(-6*time.Hour))
	integration.LinkToCompany(t, e, retired, archived)
	if _, err := e.Contacts.ArchiveCompany(e.Admin(), ids.From[ids.CompanyKind](archived), nil); err != nil {
		t.Fatalf("archiving the company: %v", err)
	}

	view, err := company360Service(e).Assemble(rep, ids.From[ids.CompanyKind](company))
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	strip := view.StateStrip
	if strip == nil || strip.Engagement == nil || strip.Engagement.LastInboundAt == nil || strip.Engagement.LastOutboundAt == nil {
		t.Fatalf("the page's engagement strip is %+v, want both dates for an account written both ways", strip)
	}

	asked := []ids.UUID{company, private, archived}
	touched := lastTouchFor(rep, t, e, asked)
	if len(touched) != 1 {
		t.Fatalf("the rep's set read answered %d companies, want only their live account: %+v", len(touched), touched)
	}
	got, ok := touched[ids.From[ids.CompanyKind](company)]
	if !ok {
		t.Fatal("the set read left out the rep's own account")
	}
	assertSameInstant(t, "set read last inbound", got.InboundAt, *strip.Engagement.LastInboundAt)
	assertSameInstant(t, "set read last outbound", got.OutboundAt, *strip.Engagement.LastOutboundAt)

	// The private account is absent because the rep may not read it, not
	// because nothing happened there; the archived one is absent for everybody.
	holder := lastTouchFor(e.As(e.Rep3, []ids.UUID{e.Team2}, integration.AccountRepPerms), t, e, asked)
	if _, ok := holder[ids.From[ids.CompanyKind](private)]; !ok {
		t.Error("the private account's holder is refused it too, so the rep's absence proves nothing")
	}
	if _, ok := lastTouchFor(e.Admin(), t, e, asked)[ids.From[ids.CompanyKind](archived)]; ok {
		t.Error("the set read answers an archived company")
	}
}

// A caller who may not read activity is refused outright, which is what the
// queue reports as a withheld pair rather than an account nobody wrote to.
func TestTheSetLastTouchRefusesAReaderWithoutTheActivityGrant(t *testing.T) {
	e := integration.Setup(t)
	company := e.SeedCompany(t, "Scale Commerce", &e.Rep1)
	noActivity := principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects:  map[string]principal.ObjectGrant{"company": {Read: true}, "contact": {Read: true}},
		RowScope: principal.RowScopeTeam,
	}
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, noActivity)
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := company360svc.LastTouchFor(ctx, tx, []ids.CompanyID{ids.From[ids.CompanyKind](company)}, company360Clock, company360svc.AssembleOptions{})
		return err
	})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a reader without activity read got %v, want a permission refusal", err)
	}
}

func lastTouchFor(ctx context.Context, t *testing.T, e *integration.Env, companies []ids.UUID) map[ids.CompanyID]company360svc.LastTouch {
	t.Helper()
	wanted := make([]ids.CompanyID, 0, len(companies))
	for _, id := range companies {
		wanted = append(wanted, ids.From[ids.CompanyKind](id))
	}
	var out map[ids.CompanyID]company360svc.LastTouch
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		touched, err := company360svc.LastTouchFor(ctx, tx, wanted, company360Clock, company360svc.AssembleOptions{})
		out = touched
		return err
	}); err != nil {
		t.Fatalf("reading the accounts' last touch: %v", err)
	}
	return out
}

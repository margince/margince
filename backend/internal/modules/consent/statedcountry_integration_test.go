// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// statedEnv is a contact under a pack with no legitimate-interest basis, who
// wrote on a thread the rep now answers: unconfirmed, the reply is refused.
func statedEnv(t *testing.T) (*resolveEnv, packPair) {
	t.Helper()
	p := registerPackPair(t)
	e := setupResolve(t)
	placeContact(t, e.owner, e.contact, p.recipient)
	e.capturedMail(t, "thread-matter", "cc", true)
	e.ctx = withActivityCreate(e.ctx)
	return e, p
}

func (e *resolveEnv) replyStating(stated ...commsauthz.CountryOverride) commsauthz.Request {
	return commsauthz.Request{
		Recipients: []connector.Recipient{{Email: e.address}}, ThreadKey: "thread-matter",
		Subject: "Re: matter", Body: "Thanks.", Evidence: commsauthz.Evidence{RecipientCountries: stated},
	}
}

func (e *resolveEnv) stageStated(ctx context.Context, t *testing.T, delivery ids.UUID, req commsauthz.Request) (commsauthz.Decision, error) {
	t.Helper()
	var set commsauthz.DecisionSet
	err := e.store.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		set, err = e.gate.AuthorizeStagingTx(ctx, tx, delivery, req)
		return err
	})
	if err != nil {
		return commsauthz.Decision{}, err
	}
	return set.Decisions[0], nil
}

type recordedCountry struct {
	source *string
	codes  []string
}

func (e *resolveEnv) recordedCountry(t *testing.T, delivery ids.UUID, phase string) recordedCountry {
	t.Helper()
	var out recordedCountry
	if err := e.owner.QueryRow(context.Background(), `
		SELECT country_source, ruleset_codes FROM communication_decision
		 WHERE delivery_id = $1 AND phase = $2`, delivery, phase).Scan(&out.source, &out.codes); err != nil {
		t.Fatalf("reading the %s decision: %v", phase, err)
	}
	return out
}

// A stated or cleared country decides staging and transmit, and both rows say
// it came from the sender.
func TestAStatedCountryDecidesStagingAndTransmit(t *testing.T) {
	for name, c := range map[string]struct {
		stated func(packPair) string
		source string
		codes  func(packPair) []string
	}{
		"stated": {func(p packPair) string { return string(p.installation) }, string(CountryFromSender),
			func(p packPair) []string { return []string{string(p.installation)} }},
		"cleared": {func(packPair) string { return "" }, string(CountryClearedBySender),
			func(packPair) []string { return nil }},
	} {
		t.Run(name, func(t *testing.T) {
			e, p := statedEnv(t)
			if d, err := e.stageStated(e.ctx, t, e.plantDelivery(t), e.replyStating()); err != nil ||
				d.ReasonCode != commsauthz.ReasonLawfulBasisUnconfirmed {
				t.Fatalf("with the record's country the reply staged as %+v (%v), want refused as unconfirmed", d, err)
			}
			delivery := e.deliveryOnThread(t, "thread-matter")
			stated := commsauthz.CountryOverride{Address: e.address, Country: c.stated(p)}
			d, err := e.stageStated(e.ctx, t, delivery, e.replyStating(stated))
			if err != nil || d.Verdict != commsauthz.VerdictAllow {
				t.Fatalf("with the country %s the reply staged as %+v (%v), want allowed", name, d, err)
			}
			ticket, err := e.gate.AuthorizeTransmit(e.ctx, commsauthz.TransmitRequest{
				DeliveryID: delivery, Attempt: 1, Recipients: []connector.Recipient{{Email: e.address}},
				Subject: "Re: matter", Body: "Thanks.",
			})
			if err != nil || !ticket.Allowed {
				t.Fatalf("transmit = %+v (%v), want allowed under the staged statement", ticket, err)
			}
			for _, phase := range []string{"staging", "transmit"} {
				got := e.recordedCountry(t, delivery, phase)
				if got.source == nil || *got.source != c.source || !equalCodes(got.codes, c.codes(p)) {
					t.Errorf("%s row: source %v codes %v, want %s %v", phase, got.source, got.codes, c.source, c.codes(p))
				}
			}
		})
	}
}

func equalCodes(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// An agent may not move a recipient's jurisdiction: the preview and staging
// refuse, and nothing is staged.
func TestAnAgentMayNotStateARecipientsCountry(t *testing.T) {
	e, p := statedEnv(t)
	agent := principal.WithActor(e.ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:x", OnBehalfOf: e.user,
		Permissions: principal.Permissions{RoleKeys: []string{"admin"}, RowScope: principal.RowScopeAll},
	})
	req := e.replyStating(commsauthz.CountryOverride{Address: e.address, Country: string(p.installation)})
	var refused *StatedCountryError
	if _, err := e.gate.Preview(agent, req); !errors.As(err, &refused) {
		t.Errorf("an agent's preview with a stated country = %v, want refused", err)
	}
	delivery := e.plantDelivery(t)
	if _, err := e.stageStated(agent, t, delivery, req); !errors.As(err, &refused) {
		t.Fatalf("an agent's staging with a stated country = %v, want refused", err)
	}
	var rows int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM communication_decision WHERE delivery_id = $1`, delivery).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("the refused staging left %d decision rows", rows)
	}
	if _, err := e.gate.Preview(e.ctx, req); err != nil {
		t.Errorf("the rep's own preview with the same statement = %v, want answered", err)
	}
}

// Clearing a country moves no objection. A recipient who asked us to stop is
// refused alike with the country stated, cleared or left to the record.
func TestAClearedCountryKeepsALiveObjection(t *testing.T) {
	e, p := statedEnv(t)
	e.suppress(t, "subject_request")
	for name, stated := range map[string][]commsauthz.CountryOverride{
		"the record's": nil,
		"stated":       {{Address: e.address, Country: string(p.installation)}},
		"cleared":      {{Address: e.address}},
	} {
		d, err := e.stageStated(e.ctx, t, e.plantDelivery(t), e.replyStating(stated...))
		if err != nil || d.ReasonCode != commsauthz.ReasonSubjectRequest {
			t.Errorf("%s country: staged as %+v (%v), want the objection to refuse", name, d, err)
		}
	}
}

// Clearing a recipient's country removes the duties their country owes and
// keeps the ones the sender's own country owes.
func TestAClearedCountryKeepsTheSendersDuties(t *testing.T) {
	e := setupChannelConsent(t)
	code := testMessagingRules(t, messagingrules.Rules{
		Version: 1,
		Disclosures: []messagingrules.Disclosure{
			{Kind: messagingrules.ControllerIdentity},
			{Kind: messagingrules.ObjectionRoute, MarketingOnly: true},
		},
	})
	stateParticulars(t, e, ControllerParticulars{
		LegalName: "Beispiel GmbH", PostalAddress: "Hauptstraße 1", ObjectionRoute: "Reply STOP.",
	})
	livesIn(t, e, string(code))
	store := e.store.WithInstallationCountry(
		InstallationCountryFunc(func(context.Context, pgx.Tx) (jurisdiction.Code, error) { return code, nil }))
	kinds := func(stated []commsauthz.CountryOverride) []string {
		var out []string
		if err := store.db.Tx(e.ctx, func(tx pgx.Tx) error {
			lines, err := store.DisclosuresFor(e.ctx, tx, DisclosureQuery{
				Category: commsauthz.CategoryMarketing, Recipients: []string{recipientAddress(e)}, Countries: stated,
			})
			for _, line := range lines {
				out = append(out, line.Kind)
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return out
	}
	if got := kinds(nil); len(got) != 2 {
		t.Fatalf("under the record's country owed %v, want the controller identity and the objection route", got)
	}
	got := kinds([]commsauthz.CountryOverride{{Address: recipientAddress(e)}})
	if len(got) != 1 || got[0] != string(messagingrules.ControllerIdentity) {
		t.Errorf("with the recipient's country cleared owed %v, want only the sender's controller identity", got)
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// Which law a refusal or finding names, as the sender reads it. Each recipient
// is cited from their own country's pack, and only while it may be shown. A
// review keeps the citation it was assessed with.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// citedAt is the engine's clock in these tests, so a pack's dates fall the
// same side of it on every run.
var citedAt = time.Date(2026, time.June, 1, 9, 0, 0, 0, time.UTC)

// kaiAddress is the second recipient, who lives in another market than Dana.
const kaiAddress = "kai@buyer.test"

// lawOf is one market's law for the objection and its missing footer line.
type lawOf struct {
	act, refusal, footer, source string
}

func (l lawOf) instrument() messagingrules.Instrument {
	return messagingrules.Instrument{Name: l.act, SourceURL: l.source}
}

func (l lawOf) refusalProvision() commsauthz.Provision {
	return commsauthz.Provision{Instrument: l.act, Provision: l.refusal, SourceURL: l.source}
}

func (l lawOf) footerProvision() commsauthz.Provision {
	return commsauthz.Provision{Instrument: l.act, Provision: l.footer, SourceURL: l.source}
}

var (
	germanLaw     = lawOf{act: "Act on unfair competition", refusal: "§ 7(2)", footer: "§ 7(3)", source: "https://example.org/uwg"}
	vietnameseLaw = lawOf{act: "Decree on advertising", refusal: "Art. 21", footer: "Art. 23", source: "https://example.org/decree"}
	reviewedOnce  = messagingrules.Provenance{
		SourceCheckedAt: citedAt.AddDate(0, -2, 0), ReviewedAt: citedAt.AddDate(0, -1, 0),
		ReviewDue: citedAt.AddDate(1, 0, 0), Reviewer: "pack maintainers",
	}
)

// citingPack registers a pack that cites law for the objection and for the
// missing objection route, under the given review and instrument dates.
func citingPack(t *testing.T, law lawOf, review messagingrules.Provenance, instrument messagingrules.Instrument) jurisdiction.Code {
	t.Helper()
	return testMessagingRules(t, messagingrules.Rules{
		Version: 1, Instruments: []messagingrules.Instrument{instrument}, Provenance: review,
		Citations: []messagingrules.Citation{
			{Code: commsauthz.ReasonObjection, Instrument: law.act, Provision: law.refusal},
			{Code: string(messagingrules.ObjectionRoute), Instrument: law.act, Provision: law.footer},
		},
	})
}

// liveIn puts the contact behind address in code's country, the way the
// contacts writer stores one.
func (e *resolveEnv) liveIn(t *testing.T, address string, code jurisdiction.Code) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), `
		UPDATE contact SET address_country = upper($2)
		 WHERE id = (SELECT contact_id FROM contact_email WHERE email = $1)`, address, string(code)); err != nil {
		t.Fatal(err)
	}
}

// citedEnv is Dana, who objected to marketing, on the engine's fixed clock.
func citedEnv(t *testing.T) *resolveEnv {
	t.Helper()
	e := setupResolve(t)
	e.store.now = func() time.Time { return citedAt }
	e.seedPurpose(t, "newsletter", "marketing")
	e.suppress(t, "marketing_objection")
	return e
}

// refuseNewsletter stages a newsletter whose footer lacks the objection route
// and records the review the refusal opens.
func (e *resolveEnv) refuseNewsletter(t *testing.T, recipients ...string) Review {
	t.Helper()
	to := make([]connector.Recipient, 0, len(recipients))
	for _, address := range recipients {
		to = append(to, connector.Recipient{Email: address})
	}
	var set commsauthz.DecisionSet
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		set, err = e.gate.AuthorizeStagingTx(e.ctx, tx, e.plantDelivery(t), commsauthz.Request{
			Recipients: to, ThreadKey: "kai-question", LegacyPurposeKey: "newsletter",
			Evidence: commsauthz.Evidence{FooterFindings: []commsauthz.FooterFinding{{Kind: string(messagingrules.ObjectionRoute)}}},
			Subject:  "S", Body: "b",
		})
		return err
	}); err != nil {
		t.Fatalf("staging: %v", err)
	}
	review, err := e.gate.RecordRefusal(e.ctx, set, ids.UUID{})
	if err != nil {
		t.Fatalf("recording the refusal: %v", err)
	}
	return review
}

func refusalFor(t *testing.T, review Review, address string) RefusedRecipient {
	t.Helper()
	for _, refusal := range review.Refusals {
		if refusal.Address == address {
			return refusal
		}
	}
	t.Fatalf("the review names no refusal for %s: %+v", address, review.Refusals)
	return RefusedRecipient{}
}

// One message to a German and a Vietnamese recipient: each refusal and each
// footer finding names the law of that recipient's own market.
func TestEachRecipientIsCitedFromTheirOwnMarket(t *testing.T) {
	e := citedEnv(t)
	e.secondContactWhoWroteAMonthAgo(t, kaiAddress)
	e.liveIn(t, e.address, citingPack(t, germanLaw, reviewedOnce, germanLaw.instrument()))
	e.liveIn(t, kaiAddress, citingPack(t, vietnameseLaw, reviewedOnce, vietnameseLaw.instrument()))

	review := e.refuseNewsletter(t, kaiAddress, e.address)

	dana := refusalFor(t, review, e.address)
	if dana.ReasonCode != commsauthz.ReasonObjection {
		t.Fatalf("Dana was refused for %q, want the objection: the fixture proves nothing", dana.ReasonCode)
	}
	if dana.Provision != germanLaw.refusalProvision() {
		t.Errorf("Dana's objection cites %+v, want her own market's %+v", dana.Provision, germanLaw.refusalProvision())
	}
	if got := dana.FooterFindings[0].Provision; got != germanLaw.footerProvision() {
		t.Errorf("Dana's missing objection route cites %+v, want %+v", got, germanLaw.footerProvision())
	}
	kai := refusalFor(t, review, kaiAddress)
	if got := kai.FooterFindings[0].Provision; got != vietnameseLaw.footerProvision() {
		t.Errorf("Kai's missing objection route cites %+v, want his own market's %+v", got, vietnameseLaw.footerProvision())
	}
	if kai.Provision == dana.Provision {
		t.Errorf("Kai's refusal %q cites Dana's law %+v", kai.ReasonCode, kai.Provision)
	}

	served := wireReview(review)
	wired := served.Refusals[0]
	if wired.Address != e.address || wired.Provision == nil || wired.Provision.Provision != germanLaw.refusal ||
		wired.Provision.SourceUrl != germanLaw.refusalProvision().SourceURL {
		t.Errorf("the served review's first refusal is %+v with provision %+v, want Dana's German law", wired.Address, wired.Provision)
	}
	if footer := (*wired.FooterFindings)[0].Provision; footer == nil || footer.Provision != germanLaw.footer {
		t.Errorf("the served footer finding cites %+v, want %q", footer, germanLaw.footer)
	}
}

// A citation that may not be shown leaves the same refusal with no provision,
// so the sender reads the facts and no law.
func TestACitationThatMayNotBeShownLeavesTheRefusalUncited(t *testing.T) {
	overdue := reviewedOnce
	overdue.SourceCheckedAt, overdue.ReviewedAt, overdue.ReviewDue =
		citedAt.AddDate(-2, 0, 0), citedAt.AddDate(-2, 0, 0), citedAt.AddDate(0, 0, -1)
	unreviewed := reviewedOnce
	unreviewed.ReviewedAt, unreviewed.ReviewDue = citedAt.AddDate(0, 0, 1), citedAt.AddDate(1, 0, 1)
	expired := germanLaw.instrument()
	expired.EffectiveFrom, expired.ExpiresFrom = citedAt.AddDate(-5, 0, 0), citedAt.AddDate(0, 0, -1)
	ahead := germanLaw.instrument()
	ahead.EffectiveFrom = citedAt.AddDate(1, 0, 0)
	for _, tc := range []struct {
		name string
		pack func(t *testing.T) jurisdiction.Code
		want commsauthz.Provision
	}{
		{"a reviewed pack shows its law", func(t *testing.T) jurisdiction.Code {
			return citingPack(t, germanLaw, reviewedOnce, germanLaw.instrument())
		}, germanLaw.refusalProvision()},
		{"a review that fell due", func(t *testing.T) jurisdiction.Code {
			return citingPack(t, germanLaw, overdue, germanLaw.instrument())
		}, commsauthz.Provision{}},
		{"a review dated after the send", func(t *testing.T) jurisdiction.Code {
			return citingPack(t, germanLaw, unreviewed, germanLaw.instrument())
		}, commsauthz.Provision{}},
		{"a law that expired", func(t *testing.T) jurisdiction.Code {
			return citingPack(t, germanLaw, reviewedOnce, expired)
		}, commsauthz.Provision{}},
		{"a law not yet in force", func(t *testing.T) jurisdiction.Code {
			return citingPack(t, germanLaw, reviewedOnce, ahead)
		}, commsauthz.Provision{}},
		{"a pack that cites nothing", func(t *testing.T) jurisdiction.Code {
			return testMessagingRules(t, messagingrules.Rules{Version: 2})
		}, commsauthz.Provision{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := citedEnv(t)
			e.liveIn(t, e.address, tc.pack(t))

			refusal := refusalFor(t, e.refuseNewsletter(t, e.address), e.address)
			if refusal.ReasonCode != commsauthz.ReasonObjection {
				t.Fatalf("refused for %q, want the objection whatever the citation", refusal.ReasonCode)
			}
			if refusal.Provision != tc.want {
				t.Errorf("the refusal cites %+v, want %+v", refusal.Provision, tc.want)
			}
			if footer := refusal.FooterFindings[0].Provision; tc.want.Cited() != footer.Cited() {
				t.Errorf("the footer finding cites %+v; a citation is shown for it exactly when the refusal's is", footer)
			}
		})
	}
}

// A review keeps the law it was assessed with. Since then the pack fell due
// and the recipient moved to a market with other law. The review still names
// the first law and no other.
func TestAReviewKeepsTheCitationItWasAssessedWith(t *testing.T) {
	e := citedEnv(t)
	e.liveIn(t, e.address, citingPack(t, germanLaw, reviewedOnce, germanLaw.instrument()))
	assessed := e.refuseNewsletter(t, e.address)

	e.liveIn(t, e.address, citingPack(t, vietnameseLaw, reviewedOnce, vietnameseLaw.instrument()))
	e.store.now = func() time.Time { return reviewedOnce.ReviewDue.AddDate(0, 1, 0) }
	read, err := e.store.ReviewForReader(e.ctx, assessed.ID)
	if err != nil {
		t.Fatalf("reading the review back: %v", err)
	}
	refusal := refusalFor(t, read, e.address)
	if refusal.Provision != germanLaw.refusalProvision() {
		t.Errorf("the review read back cites %+v, want the law it was assessed with %+v", refusal.Provision, germanLaw.refusalProvision())
	}
	if refusal.FooterFindings[0].Provision != germanLaw.footerProvision() {
		t.Errorf("the footer finding read back cites %+v, want %+v", refusal.FooterFindings[0].Provision, germanLaw.footerProvision())
	}
	if read.AssessmentRevision() != assessed.AssessmentRevision() {
		t.Error("the stored review's revision moved on a read, so a sender's confirmation would go stale with no change")
	}
}

// The draft check cites each recipient's pack for its own finding.
func TestADraftFindingCitesTheRecipientsOwnMarket(t *testing.T) {
	e := citedEnv(t)
	e.secondContactWhoWroteAMonthAgo(t, kaiAddress)
	foreignAdvertiser := func(law lawOf) jurisdiction.Code {
		return testMessagingRules(t, messagingrules.Rules{
			Version: 1, ForeignAdvertiserNeedsLocalAgency: true,
			Instruments: []messagingrules.Instrument{law.instrument()}, Provenance: reviewedOnce,
			Citations: []messagingrules.Citation{{
				Code: DraftForeignAdvertiser, Territory: messagingrules.SenderElsewhere, Instrument: law.act, Provision: law.refusal,
			}},
		})
	}
	german, vietnamese := foreignAdvertiser(germanLaw), foreignAdvertiser(vietnameseLaw)
	e.liveIn(t, e.address, german)
	e.liveIn(t, kaiAddress, vietnamese)

	found, err := e.store.CheckAdvertisingDraft(withActivityCreate(e.ctx), []string{e.address, kaiAddress}, "Offer", "Dear customer,\n\nOur offer.")
	if err != nil {
		t.Fatalf("checking the draft: %v", err)
	}
	want := map[string]commsauthz.Provision{
		string(german): germanLaw.refusalProvision(), string(vietnamese): vietnameseLaw.refusalProvision(),
	}
	if len(found) != len(want) {
		t.Fatalf("findings %+v, want one foreign-advertiser notice per market", found)
	}
	for _, f := range found {
		if f.Requirement != DraftForeignAdvertiser || f.Provision != want[f.Country] {
			t.Errorf("the %s finding %q cites %+v, want %+v", f.Country, f.Requirement, f.Provision, want[f.Country])
		}
	}
}

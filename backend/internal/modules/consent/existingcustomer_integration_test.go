// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// Row 8 of the approved send rules: the existing-customer route. Where the
// recipient's country grants it, a marketing send without consent goes out to
// a customer when four things hold. The customer bought from the sending
// entity. An admin declared the notice given when that sale collected the
// address. The declaration names the purpose. The customer has not objected.
// Each case below takes one condition away.

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
	"github.com/margince/margince/backend/pkg/extension/messaging"
)

const year = 365 * 24 * time.Hour

// customerRoute is a pack that grants the existing-customer route on the four
// conditions the German pack names, under a code no real pack claims.
var customerRoute = sync.OnceValue(func() string {
	messagingrules.Register(messagingrules.Rules{Jurisdiction: "zx", Version: 1,
		MarketingExceptions: []messaging.MarketingException{{
			Kind: messaging.ExistingCustomer, RequiresSaleEvidence: true,
			RequiresCollectionTimeOptOut: true, RequiresSimilarity: true, RequiresNoObjection: true,
		}}})
	return "zx"
})

// consentAdminCtx is a signed-in admin who may configure consent.
func (e *resolveEnv) consentAdminCtx() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.user.String(), UserID: e.user,
		Permissions: principal.Permissions{
			RoleKeys: []string{"admin"},
			Objects: map[string]principal.ObjectGrant{
				"consent_config": {Create: true, Read: true, Update: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

func (e *resolveEnv) purposeID(t *testing.T, key string) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := e.owner.QueryRow(context.Background(),
		`SELECT id FROM consent_purpose WHERE key = $1`, key).Scan(&id); err != nil {
		t.Fatalf("reading purpose %s: %v", key, err)
	}
	return id
}

func (e *resolveEnv) defaultEntity(t *testing.T) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := e.owner.QueryRow(context.Background(),
		`SELECT id FROM legal_entity WHERE is_default`).Scan(&id); err != nil {
		t.Fatalf("reading the default entity: %v", err)
	}
	return id
}

func (e *resolveEnv) archivedEntity(t *testing.T) ids.UUID {
	t.Helper()
	entity := e.secondEntity(t)
	e.ownerExec(t, `UPDATE legal_entity SET archived_at = now() WHERE id = $1`, entity)
	return entity
}

// saleAt plants a sale: a deal with the env's contact as its economic buyer
// from before, and an offer of entity accepted at a moment.
func (e *resolveEnv) saleAt(t *testing.T, entity ids.UUID, at time.Time) (deal, offer ids.UUID) {
	t.Helper()
	deal = ids.NewV7()
	e.ownerExec(t, `
		INSERT INTO deal (id, name, pipeline_id, stage_id, status, source, captured_by)
		VALUES ($1, 'Spring order', $2, $3, 'open', 'manual', 'human:x')`, deal, e.pipeline, e.stage)
	e.ownerExec(t, `
		INSERT INTO relationship (kind, deal_id, contact_id, role, source, captured_by, created_at)
		VALUES ('deal_stakeholder', $1, $2, 'economic_buyer', 'manual', 'human:x', $3)`,
		deal, e.contact, at.Add(-30*24*time.Hour))
	offer = e.offerOn(t, deal, "accepted")
	e.ownerExec(t, `UPDATE offer SET legal_entity_id = $2, accepted_at = $3 WHERE id = $1`, offer, entity, at)
	return deal, offer
}

// declare records a declaration through the store's door.
func (e *resolveEnv) declare(t *testing.T, entity ids.UUID, from time.Time, until *time.Time, purposes ...ids.UUID) CustomerNotice {
	t.Helper()
	n, err := e.store.DeclareCustomerNotice(e.consentAdminCtx(), DeclareNoticeInput{
		LegalEntityID: entity, CustomerGroup: "Web shop buyers", CollectionChannel: "web shop checkout",
		NoticeText:    "We will send you offers for similar products. You can object at any time, free of charge.",
		EffectiveFrom: from, EffectiveUntil: until, PurposeIDs: purposes, Now: clocktest.Now(t),
		EvidenceReference: "archived checkout page", EvidenceDatedAt: &from,
	})
	if err != nil {
		t.Fatalf("declaring the notice: %v", err)
	}
	return n
}

// customerEnv is a contact under the route who bought from the default entity
// two years ago, and a newsletter purpose, with nothing declared yet.
func customerEnv(t *testing.T) (*resolveEnv, ids.UUID, time.Time) {
	t.Helper()
	e := setupResolve(t)
	e.livesIn(t, customerRoute())
	e.seedPurpose(t, "newsletter", "marketing")
	soldAt := clocktest.Now(t).Add(-2 * year)
	e.saleAt(t, e.defaultEntity(t), soldAt)
	return e, e.purposeID(t, "newsletter"), soldAt
}

var newsletterSend = commsauthz.Request{LegacyPurposeKey: "newsletter"}

func TestADeclaredNoticeCarriesMarketingToAnExistingCustomer(t *testing.T) {
	e, newsletter, _ := customerEnv(t)
	if got := e.decide(t, newsletterSend); got.Verdict == commsauthz.VerdictAllow {
		t.Fatalf("verdict = allow before any declaration: the fixture proves nothing")
	}
	e.declare(t, e.defaultEntity(t), clocktest.Now(t).Add(-3*year), nil, newsletter)

	got := e.decide(t, newsletterSend)
	if got.Verdict != commsauthz.VerdictAllow {
		t.Fatalf("verdict = %q (%s), want allow on the existing-customer route", got.Verdict, got.ReasonCode)
	}
	if got.Basis != commsauthz.BasisExistingCustomerException {
		t.Errorf("basis = %q, want %q", got.Basis, commsauthz.BasisExistingCustomerException)
	}
}

type customerCase struct {
	name  string
	setup func(t *testing.T, e *resolveEnv, newsletter ids.UUID, soldAt time.Time)
}

var customerConditions = []customerCase{
	{"no declaration", func(*testing.T, *resolveEnv, ids.UUID, time.Time) {}},
	{"the notice started after the sale", func(t *testing.T, e *resolveEnv, newsletter ids.UUID, soldAt time.Time) {
		e.declare(t, e.defaultEntity(t), soldAt.Add(24*time.Hour), nil, newsletter)
	}},
	{"the notice ended before the sale", func(t *testing.T, e *resolveEnv, newsletter ids.UUID, soldAt time.Time) {
		until := soldAt.Add(-24 * time.Hour)
		e.declare(t, e.defaultEntity(t), soldAt.Add(-year), &until, newsletter)
	}},
	{"another purpose", func(t *testing.T, e *resolveEnv, _ ids.UUID, soldAt time.Time) {
		e.seedPurpose(t, "events", "marketing")
		e.declare(t, e.defaultEntity(t), soldAt.Add(-year), nil, e.purposeID(t, "events"))
	}},
	{"another entity", func(t *testing.T, e *resolveEnv, newsletter ids.UUID, soldAt time.Time) {
		e.declare(t, e.secondEntity(t), soldAt.Add(-year), nil, newsletter)
	}},
	{"the customer objected", func(t *testing.T, e *resolveEnv, newsletter ids.UUID, soldAt time.Time) {
		e.declare(t, e.defaultEntity(t), soldAt.Add(-year), nil, newsletter)
		e.suppress(t, "marketing_objection")
	}},
	{"the declaration was withdrawn", func(t *testing.T, e *resolveEnv, newsletter ids.UUID, soldAt time.Time) {
		n := e.declare(t, e.defaultEntity(t), soldAt.Add(-year), nil, newsletter)
		if _, err := e.store.WithdrawCustomerNotice(e.consentAdminCtx(), n.ID, "the checkout never showed it"); err != nil {
			t.Fatalf("withdrawing: %v", err)
		}
	}},
	{"the recipient's country grants no route", func(t *testing.T, e *resolveEnv, newsletter ids.UUID, soldAt time.Time) {
		e.declare(t, e.defaultEntity(t), soldAt.Add(-year), nil, newsletter)
		e.livesIn(t, "zw")
	}},
}

func TestEachExistingCustomerConditionIsNeeded(t *testing.T) {
	for _, tc := range customerConditions {
		t.Run(tc.name, func(t *testing.T) {
			e, newsletter, soldAt := customerEnv(t)
			tc.setup(t, e, newsletter, soldAt)
			got := e.decide(t, newsletterSend)
			if got.Verdict == commsauthz.VerdictAllow || got.Basis == commsauthz.BasisExistingCustomerException {
				t.Fatalf("verdict = %q on basis %q, want a refusal", got.Verdict, got.Basis)
			}
		})
	}
}

// The sale is an accepted offer of the sending entity. No case below has one
// that counts.
func TestASaleIsAnAcceptedOfferOfTheSendingEntity(t *testing.T) {
	for name, plant := range map[string]func(t *testing.T, e *resolveEnv, at time.Time){
		"a won deal with no accepted offer": func(t *testing.T, e *resolveEnv, at time.Time) {
			deal, offer := e.saleAt(t, e.defaultEntity(t), at)
			e.ownerExec(t, `UPDATE deal SET status = 'won', closed_at = $2 WHERE id = $1`, deal, at)
			e.ownerExec(t, `UPDATE offer SET status = 'sent', accepted_at = NULL WHERE id = $1`, offer)
		},
		"an offer the branch issued": func(t *testing.T, e *resolveEnv, at time.Time) {
			e.saleAt(t, e.secondEntity(t), at)
		},
		"an offer that names no issuer": func(t *testing.T, e *resolveEnv, at time.Time) {
			_, offer := e.saleAt(t, e.defaultEntity(t), at)
			e.ownerExec(t, `UPDATE offer SET legal_entity_id = NULL WHERE id = $1`, offer)
		},
		"a stakeholder added after the acceptance": func(t *testing.T, e *resolveEnv, at time.Time) {
			deal, _ := e.saleAt(t, e.defaultEntity(t), at)
			e.ownerExec(t, `UPDATE relationship SET created_at = $2 WHERE deal_id = $1`, deal, at.Add(time.Hour))
		},
		"a stakeholder who only advised": func(t *testing.T, e *resolveEnv, at time.Time) {
			deal, _ := e.saleAt(t, e.defaultEntity(t), at)
			e.ownerExec(t, `UPDATE relationship SET role = 'influencer' WHERE deal_id = $1`, deal)
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := setupResolve(t)
			e.livesIn(t, customerRoute())
			e.seedPurpose(t, "newsletter", "marketing")
			at := clocktest.Now(t).Add(-year)
			plant(t, e, at)
			e.declare(t, e.defaultEntity(t), at.Add(-year), nil, e.purposeID(t, "newsletter"))
			if got := e.decide(t, newsletterSend); got.Verdict == commsauthz.VerdictAllow {
				t.Fatalf("verdict = allow on basis %q, want a refusal", got.Basis)
			}
		})
	}
}

func (e *resolveEnv) marketingVerdict(t *testing.T, marketing MarketingContext) Verdict {
	t.Helper()
	purpose := PurposeRow{ID: e.purposeID(t, "newsletter").String(), Key: "newsletter", Label: "Newsletter", Class: ClassMarketing}
	var out Verdict
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		out, err = GuardVerdictForContact(e.ctx, tx, e.contact.String(), purpose, time.Now().Add(-defaultReplyWindow), marketing)
		return err
	}); err != nil {
		t.Fatalf("reading the verdict: %v", err)
	}
	return out
}

func TestTheVerdictNamesTheSaleItAllowedOn(t *testing.T) {
	e, newsletter, soldAt := customerEnv(t)
	n := e.declare(t, e.defaultEntity(t), soldAt.Add(-year), nil, newsletter)
	de := messaging.MarketingException{Kind: messaging.ExistingCustomer, RequiresSaleEvidence: true,
		RequiresCollectionTimeOptOut: true, RequiresSimilarity: true, RequiresNoObjection: true}
	got := e.marketingVerdict(t, MarketingContext{Exception: &de, SendingEntity: e.defaultEntity(t)})
	if got.State != VerdictAllowed || got.ExistingCustomer == nil || got.ExistingCustomer.NoticeID != n.ID {
		t.Fatalf("verdict = %+v, want allowed on declaration %s", got, n.ID)
	}
	none := e.marketingVerdict(t, MarketingContext{})
	if none.State != VerdictUnknown || none.ReasonCode != ReasonNoConsent {
		t.Fatalf("verdict with no exception = %+v, want unknown with the plain reason", none)
	}
}

func TestOnlyASignedInConsentAdminDeclaresANotice(t *testing.T) {
	e, newsletter, _ := customerEnv(t)
	in := DeclareNoticeInput{
		LegalEntityID: e.defaultEntity(t), CustomerGroup: "Buyers", CollectionChannel: "order form",
		NoticeText: "You may object.", EffectiveFrom: clocktest.Now(t).Add(-year),
		PurposeIDs: []ids.UUID{newsletter}, Now: clocktest.Now(t),
	}
	if _, err := e.store.DeclareCustomerNotice(e.seatCtx(e.user), in); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a rep declared a notice: err = %v, want permission denied", err)
	}
	actor, ok := principal.Actor(e.consentAdminCtx())
	if !ok {
		t.Fatal("the admin context carries no actor")
	}
	actor.Type, actor.OnBehalfOf = principal.PrincipalAgent, e.user
	agent := principal.WithActor(e.consentAdminCtx(), actor)
	if _, err := e.store.DeclareCustomerNotice(agent, in); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("an agent declared a notice: err = %v, want permission denied", err)
	}
}

func TestADeclarationStatesAPastPeriodAndMarketingPurposes(t *testing.T) {
	e, newsletter, _ := customerEnv(t)
	e.seedPurpose(t, "service", "transactional")
	now := clocktest.Now(t)
	base := DeclareNoticeInput{
		LegalEntityID: e.defaultEntity(t), CustomerGroup: "Buyers", CollectionChannel: "order form",
		NoticeText: "You may object.", EffectiveFrom: now.Add(-year), PurposeIDs: []ids.UUID{newsletter}, Now: now,
	}
	for name, mutate := range map[string]func(*DeclareNoticeInput){
		"a future start":      func(in *DeclareNoticeInput) { in.EffectiveFrom = now.Add(time.Hour) },
		"an end before start": func(in *DeclareNoticeInput) { end := in.EffectiveFrom; in.EffectiveUntil = &end },
		"no purpose":          func(in *DeclareNoticeInput) { in.PurposeIDs = nil },
		"a service purpose":   func(in *DeclareNoticeInput) { in.PurposeIDs = []ids.UUID{e.purposeID(t, "service")} },
		"no customer group":   func(in *DeclareNoticeInput) { in.CustomerGroup = "  " },
		"an archived entity":  func(in *DeclareNoticeInput) { in.LegalEntityID = e.archivedEntity(t) },
	} {
		in := base
		mutate(&in)
		var invalid *ValidationError
		if _, err := e.store.DeclareCustomerNotice(e.consentAdminCtx(), in); !errors.As(err, &invalid) {
			t.Errorf("%s: err = %v, want a validation error", name, err)
		}
	}

	n := e.declare(t, e.defaultEntity(t), now.Add(-5*year), nil, newsletter)
	if _, err := e.store.WithdrawCustomerNotice(e.consentAdminCtx(), n.ID, "wrong period"); err != nil {
		t.Fatalf("withdrawing: %v", err)
	}
	if _, err := e.store.WithdrawCustomerNotice(e.consentAdminCtx(), n.ID, "again"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a second withdrawal: err = %v, want not found", err)
	}
	listed, err := e.store.CustomerNotices(e.consentAdminCtx())
	if err != nil || len(listed) != 1 || listed[0].WithdrawnReason == nil || *listed[0].WithdrawnReason != "wrong period" {
		t.Fatalf("listed = %+v (%v), want the one withdrawn declaration with its reason", listed, err)
	}
}

// The buyer of a sale counts and a colleague on the same deal who only
// advised does not. A billing contact of the buyer company counts too.
func TestOnlyTheBuyerOfASaleIsItsCustomer(t *testing.T) {
	e, newsletter, soldAt := customerEnv(t)
	e.declare(t, e.defaultEntity(t), soldAt.Add(-year), nil, newsletter)
	if got := e.decide(t, newsletterSend); got.Verdict != commsauthz.VerdictAllow {
		t.Fatalf("the buyer got %q (%s), want allow", got.Verdict, got.ReasonCode)
	}
	adviser := e.anotherContact(t, "adviser@buyer.test")
	adviser.livesIn(t, customerRoute())
	e.ownerExec(t, `
		INSERT INTO relationship (kind, deal_id, contact_id, role, source, captured_by, created_at)
		SELECT 'deal_stakeholder', deal_id, $1, 'influencer', 'manual', 'human:x', created_at
		  FROM relationship WHERE contact_id = $2 AND kind = 'deal_stakeholder'`, adviser.contact, e.contact)
	if got := adviser.decide(t, newsletterSend); got.Verdict == commsauthz.VerdictAllow {
		t.Fatalf("an adviser on the buyer's deal was allowed on basis %q", got.Basis)
	}

	company := e.company(t)
	e.ownerExec(t, `UPDATE offer SET buyer_company_id = $1`, company)
	e.ownerExec(t, `
		INSERT INTO relationship (kind, company_id, contact_id, role, source, captured_by, created_at)
		VALUES ('billing_contact', $1, $2, 'recipient', 'manual', 'human:x', $3)`,
		company, adviser.contact, soldAt.Add(-time.Hour))
	if got := adviser.decide(t, newsletterSend); got.Verdict != commsauthz.VerdictAllow {
		t.Fatalf("the buyer company's billing contact got %q (%s), want allow", got.Verdict, got.ReasonCode)
	}
}

// A declaration back-dated without proof covers no sale before the day it is
// declared; the same declaration with dated proof covers it.
func TestABackDatedDeclarationNeedsDatedProof(t *testing.T) {
	e, newsletter, soldAt := customerEnv(t)
	if _, err := e.store.DeclareCustomerNotice(e.consentAdminCtx(), DeclareNoticeInput{
		LegalEntityID: e.defaultEntity(t), CustomerGroup: "Web shop buyers", CollectionChannel: "checkout",
		NoticeText: "You may object.", EffectiveFrom: soldAt.Add(-year),
		PurposeIDs: []ids.UUID{newsletter}, Now: clocktest.Now(t),
	}); err != nil {
		t.Fatalf("declaring without proof: %v", err)
	}
	if got := e.decide(t, newsletterSend); got.Verdict == commsauthz.VerdictAllow {
		t.Fatal("a declaration without proof covered a sale from before the day it was declared")
	}
	proofDate := soldAt.Add(-24 * time.Hour)
	e.declare(t, e.defaultEntity(t), soldAt.Add(-year), nil, newsletter)
	e.ownerExec(t, `UPDATE existing_customer_notice SET evidence_dated_at = $1 WHERE evidence_dated_at IS NOT NULL`, proofDate)
	if got := e.decide(t, newsletterSend); got.Verdict != commsauthz.VerdictAllow {
		t.Fatalf("with dated proof got %q (%s), want allow", got.Verdict, got.ReasonCode)
	}
	e.ownerExec(t, `UPDATE existing_customer_notice SET evidence_dated_at = $1 WHERE evidence_dated_at IS NOT NULL`, soldAt.Add(time.Hour))
	if got := e.decide(t, newsletterSend); got.Verdict == commsauthz.VerdictAllow {
		t.Fatal("proof dated after the sale covered it")
	}
}

// A message naming two different purposes is refused, whichever it is judged by.
func TestAMessageNamingTwoPurposesIsRefused(t *testing.T) {
	e, newsletter, soldAt := customerEnv(t)
	e.declare(t, e.defaultEntity(t), soldAt.Add(-year), nil, newsletter)
	e.seedPurpose(t, "events", "marketing")
	got := e.decide(t, commsauthz.Request{LegacyPurposeKey: "events", MarketingPurpose: "newsletter"})
	if got.Verdict != commsauthz.VerdictDeny || got.ReasonCode != commsauthz.ReasonClaimContradictsResolution {
		t.Fatalf("verdict = %q (%s), want a refusal for two purposes", got.Verdict, got.ReasonCode)
	}
	if got := e.decide(t, commsauthz.Request{MarketingPurpose: "newsletter"}); got.Verdict != commsauthz.VerdictAllow {
		t.Fatalf("the marketing purpose alone got %q (%s), want allow", got.Verdict, got.ReasonCode)
	}
}

// The staging row of a send on the route names the entity, the purpose, the
// declaration and the sale it rested on.
func TestTheDecisionRowNamesTheGroundItRestedOn(t *testing.T) {
	e, newsletter, soldAt := customerEnv(t)
	n := e.declare(t, e.defaultEntity(t), soldAt.Add(-year), nil, newsletter)
	delivery := e.plantDelivery(t)
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		_, err := e.gate.AuthorizeStagingTx(e.ctx, tx, delivery, commsauthz.Request{
			Recipients: []connector.Recipient{{Email: e.address}}, LegacyPurposeKey: "newsletter",
			Subject: "S", Body: "b"})
		return err
	}); err != nil {
		t.Fatalf("staging: %v", err)
	}
	var entity, notice ids.UUID
	var purpose string
	var offer *ids.UUID
	if err := e.owner.QueryRow(context.Background(), `
		SELECT legal_entity_id, purpose_key, existing_customer_notice_id, existing_customer_offer_id
		  FROM communication_decision WHERE delivery_id = $1`, delivery).Scan(&entity, &purpose, &notice, &offer); err != nil {
		t.Fatalf("reading the staging row: %v", err)
	}
	if entity != e.defaultEntity(t) || purpose != "newsletter" || notice != n.ID || offer == nil {
		t.Fatalf("row = entity %s purpose %q notice %s offer %v, want the default entity, newsletter, %s and the sale",
			entity, purpose, notice, offer, n.ID)
	}
}

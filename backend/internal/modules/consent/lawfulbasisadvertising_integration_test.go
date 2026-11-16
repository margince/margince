// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"context"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"

	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// consentOnlyCountry is one minted code under a pack shaped like the
// Vietnamese one. Consent is the only route to advertising, and no use rests
// on legitimate interests. A consent mail answers the contact's request.
var consentOnlyCountry struct {
	once sync.Once
	code jurisdiction.Code
}

func placeUnderConsentOnly(t *testing.T, e *resolveEnv) {
	t.Helper()
	consentOnlyCountry.once.Do(func() {
		consentOnlyCountry.code = testJurisdiction(t)
		messagingrules.Register(messagingrules.Rules{
			Jurisdiction: consentOnlyCountry.code, Version: 1, ConsentOnly: true,
			DataProtectionFollowsRecipient: true, DisclosureLanguage: string(mailcopy.Vietnamese),
			NoLegitimateInterests: true, ConsentConfirmationNeedsRequest: true,
			ConsentRequestFirstFor: []messagingrules.PurposeClass{messagingrules.PurposeClass(ClassMarketing)},
		})
	})
	placeContact(t, e.owner, e.contact, consentOnlyCountry.code)
}

// seedDoubleOptInPurpose plants a live purpose of class confirmed by double
// opt-in, and names it.
func seedDoubleOptInPurpose(t *testing.T, e *resolveEnv, key, class string) ids.PurposeID {
	t.Helper()
	purpose := ids.New[ids.PurposeKind]()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO consent_purpose (id, key, label, class, requires_double_opt_in)
		VALUES ($1, $2, $2, $3, true)`, purpose, key, class); err != nil {
		t.Fatalf("planting the purpose: %v", err)
	}
	return purpose
}

// Advertising to a recipient under such a pack, with no consent on record, is
// refused. It is never the warning a sender may confirm, and a confirmation
// does not let it through.
func TestAdvertisingWithoutConsentUnderThePackIsRefusedNotWarned(t *testing.T) {
	e := setupResolve(t)
	placeUnderConsentOnly(t, e)
	e.ctx = withActivityCreate(e.ctx)
	e.seedPurpose(t, "rp_offers", "marketing")

	stageAdvertising := func() commsauthz.Decision {
		t.Helper()
		delivery := e.plantDelivery(t)
		var set commsauthz.DecisionSet
		if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
			var err error
			set, err = e.gate.AuthorizeStagingTx(e.ctx, tx, delivery, commsauthz.Request{
				Recipients: []connector.Recipient{{Email: e.address}},
				Context:    commsauthz.CategoryMarketing, MarketingPurpose: "rp_offers",
				Subject: "[QC] Ưu đãi", Body: "Kính gửi quý khách.",
			})
			return err
		}); err != nil {
			t.Fatalf("staging: %v", err)
		}
		return set.Decisions[0]
	}

	refused := func(d commsauthz.Decision) {
		t.Helper()
		if d.Verdict != commsauthz.VerdictDeny || d.ReasonCode != commsauthz.ReasonMarketingConsentRequired {
			t.Errorf("advertising with no consent staged as %+v, want refused as %q",
				d, commsauthz.ReasonMarketingConsentRequired)
		}
		if d.LawfulBasisMissing {
			t.Error("advertising with no consent was marked for the warning a sender may confirm")
		}
	}
	refused(stageAdvertising())

	if _, err := e.store.ConfirmSendWithoutLawfulBasis(e.ctx,
		[]ConfirmedSubject{{Kind: entityContact, ID: e.contact.UUID}}); err != nil {
		t.Fatalf("confirming the warning: %v", err)
	}
	refused(stageAdvertising())
}

// A reply to a recipient with no basis on record names the purpose a consent
// request would ask about: the correspondence purpose, not the newsletter.
// Asked through the double-opt-in door, that request goes without a request of
// the contact's own, because the pack asks one only for advertising. Its link
// names no request, and its audit row names the user who asked. A request for
// the newsletter is still refused.
func TestANoBasisRecipientCanBeAskedForConsentToCorrespondence(t *testing.T) {
	e := setupResolve(t)
	placeUnderConsentOnly(t, e)
	e.capturedMail(t, "thread-matter", "cc", true)
	news := seedDoubleOptInPurpose(t, e, "rp_ask_news", "marketing")
	correspondence := seedDoubleOptInPurpose(t, e, "rp_ask_correspondence", "business_correspondence")

	set, err := e.gate.Preview(e.ctx, commsauthz.Request{
		Recipients: []connector.Recipient{{Email: e.address}}, ThreadKey: "thread-matter",
	})
	if err != nil {
		t.Fatalf("previewing: %v", err)
	}
	if len(set.Decisions) != 1 || !set.Decisions[0].LawfulBasisMissing {
		t.Fatalf("preview %+v, want the recipient marked", set.Decisions)
	}
	if got := set.Decisions[0].ConsentPurpose; got != correspondence.UUID {
		t.Fatalf("the preview names purpose %v, want the correspondence purpose %v", got, correspondence)
	}

	h := Handlers{store: e.store.WithConfirmationLane(&recordingStager{}, &recordingVault{}, "https://crm.example.test")}
	ctx := withContactUpdate(e.ctx)
	if rec := askConsent(t, ctx, h, e.contact, correspondence); rec.Code != http.StatusCreated {
		t.Fatalf("asking for consent to correspondence answered %d %s, want 201", rec.Code, rec.Body.String())
	}
	var tokenID ids.UUID
	var request *ids.UUID
	if err := e.owner.QueryRow(context.Background(), `
		SELECT id, subject_request_id FROM confirm_token WHERE contact_id = $1 AND purpose_id = $2`,
		e.contact, correspondence).Scan(&tokenID, &request); err != nil {
		t.Fatalf("reading the link: %v", err)
	}
	if request != nil {
		t.Errorf("the link names request %v, want none: the contact asked for nothing", *request)
	}
	var actor string
	if err := e.owner.QueryRow(context.Background(), `
		SELECT actor_id FROM audit_log WHERE entity_type = 'confirm_token' AND entity_id = $1 AND action = 'create'`,
		tokenID).Scan(&actor); err != nil {
		t.Fatalf("reading the link's audit row: %v", err)
	}
	if want := "human:" + e.user.String(); actor != want {
		t.Errorf("the link's audit row names %q, want the asking user %q", actor, want)
	}

	if rec := askConsent(t, ctx, h, e.contact, news); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("asking for consent to the newsletter answered %d %s, want 422: the contact asked for nothing",
			rec.Code, rec.Body.String())
	}
}

// With no purpose covering correspondence, the preview names no purpose and
// the composer offers no request. A newsletter does not stand in for one.
func TestNoConsentPurposeIsNamedWhenNoneCoversTheUse(t *testing.T) {
	e := setupResolve(t)
	placeUnderConsentOnly(t, e)
	e.capturedMail(t, "thread-matter", "cc", true)
	seedDoubleOptInPurpose(t, e, "rp_only_news", "marketing")

	set, err := e.gate.Preview(e.ctx, commsauthz.Request{
		Recipients: []connector.Recipient{{Email: e.address}}, ThreadKey: "thread-matter",
	})
	if err != nil {
		t.Fatalf("previewing: %v", err)
	}
	if len(set.Decisions) != 1 || !set.Decisions[0].LawfulBasisMissing {
		t.Fatalf("preview %+v, want the recipient marked", set.Decisions)
	}
	if got := set.Decisions[0].ConsentPurpose; !got.IsZero() {
		t.Errorf("the preview names purpose %v, want none: no purpose covers correspondence", got)
	}
}

// On the catalog a fresh installation seeds, a contact with no basis gets a
// consent request for the built-in correspondence purpose. Confirming it
// grants that purpose, and the next preview no longer warns.
func TestTheSeededCatalogLetsANoBasisContactBeAskedForConsent(t *testing.T) {
	e := setupResolve(t)
	placeUnderConsentOnly(t, e)
	e.capturedMail(t, "thread-matter", "cc", true)
	seedDefaultCatalog(t, e)
	var correspondence ids.UUID
	if err := e.owner.QueryRow(context.Background(),
		`SELECT id FROM consent_purpose WHERE key = $1`, string(ClassBusinessCorrespondence)).Scan(&correspondence); err != nil {
		t.Fatalf("reading the seeded correspondence purpose: %v", err)
	}
	preview := func() commsauthz.Decision {
		t.Helper()
		set, err := e.gate.Preview(e.ctx, commsauthz.Request{
			Recipients: []connector.Recipient{{Email: e.address}}, ThreadKey: "thread-matter",
		})
		if err != nil || len(set.Decisions) != 1 {
			t.Fatalf("previewing: %+v, %v", set.Decisions, err)
		}
		return set.Decisions[0]
	}
	if d := preview(); !d.LawfulBasisMissing || d.ConsentPurpose != correspondence {
		t.Fatalf("preview %+v, want the recipient marked and the correspondence purpose %v named", d, correspondence)
	}

	store := e.store.WithConfirmationLane(&recordingStager{}, &recordingVault{}, "https://crm.example.test")
	ctx := withContactUpdate(e.ctx)
	if rec := askConsent(t, ctx, Handlers{store: store}, e.contact, ids.From[ids.PurposeKind](correspondence)); rec.Code != http.StatusCreated {
		t.Fatalf("asking for consent answered %d %s, want 201", rec.Code, rec.Body.String())
	}
	issued, err := store.IssueConsentLink(ctx, e.contact, ids.From[ids.PurposeKind](correspondence), "", "")
	if err != nil {
		t.Fatalf("minting the link the contact answers: %v", err)
	}
	if _, err := store.SubmitConfirmation(ctx, issued.Token, ConfirmSubmission{
		MarketingChoice: string(StateGranted), MarketingWording: "Business correspondence.",
	}); err != nil {
		t.Fatalf("the contact confirming: %v", err)
	}
	if d := preview(); d.LawfulBasisMissing || d.Basis != commsauthz.BasisConsent {
		t.Errorf("after the contact confirmed, preview %+v, want it resting on consent and unmarked", d)
	}
}

// A marketing purpose not confirmed by double opt-in is still never asked
// about by mail.
func TestAMarketingPurposeWithoutDoubleOptInIsNotAskedByMail(t *testing.T) {
	e := setupResolve(t)
	e.seedPurpose(t, "rp_plain_news", "marketing")
	var news ids.PurposeID
	if err := e.owner.QueryRow(context.Background(),
		`SELECT id FROM consent_purpose WHERE key = 'rp_plain_news'`).Scan(&news); err != nil {
		t.Fatal(err)
	}
	h := Handlers{store: e.store.WithConfirmationLane(&recordingStager{}, &recordingVault{}, "https://crm.example.test")}
	rec := askConsent(t, withContactUpdate(e.ctx), h, e.contact, news)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "purpose_id") {
		t.Errorf("asking about a marketing purpose without double opt-in answered %d %s, want 422 naming purpose_id",
			rec.Code, rec.Body.String())
	}
}

// seedDefaultCatalog plants the purposes a fresh installation starts with,
// through the real seeder.
func seedDefaultCatalog(t *testing.T, e *resolveEnv) {
	t.Helper()
	ctx := context.Background()
	tx, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := SeedDefaultPurposesTx(ctx, tx); err != nil {
		t.Fatalf("seeding the catalog: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// askConsent drives the double-opt-in door the composer's button posts to.
func askConsent(t *testing.T, ctx context.Context, h Handlers, contact ids.ContactID, purpose ids.PurposeID) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/contacts/x/consent/double-opt-in",
		strings.NewReader(`{"purpose_id":"`+purpose.String()+`"}`)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	h.IssueDoubleOptIn(rec, req, crmcontracts.Id(contact.UUID))
	return rec
}

// withContactUpdate adds the grant a consent request asks for.
func withContactUpdate(ctx context.Context) context.Context {
	actor, _ := principal.Actor(ctx)
	grants := map[string]principal.ObjectGrant{}
	maps.Copy(grants, actor.Permissions.Objects)
	grants["contact"] = principal.ObjectGrant{Read: true, Update: true}
	actor.Permissions.Objects = grants
	return principal.WithActor(ctx, actor)
}

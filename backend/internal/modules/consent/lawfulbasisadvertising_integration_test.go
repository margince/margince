// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"context"
	"errors"
	"maps"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

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
// That request goes without a request of the contact's own, because it asks
// for no advertising. A request for the newsletter still needs one.
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

	store := e.store.WithConfirmationLane(&recordingStager{}, &recordingVault{}, "https://crm.example.test")
	ctx := withContactUpdate(e.ctx)
	if _, err := store.IssueConsentLink(ctx, e.contact, correspondence, "", ""); err != nil {
		t.Errorf("asking for consent to correspondence: %v, want the mail to go", err)
	}
	var missing *NoSubjectRequestError
	if _, err := store.IssueConsentLink(ctx, e.contact, news, "", ""); !errors.As(err, &missing) {
		t.Errorf("asking for consent to the newsletter: %v, want the request rule's refusal", err)
	}
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

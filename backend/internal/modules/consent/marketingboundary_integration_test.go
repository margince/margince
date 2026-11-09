// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"context"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/messagingrules"
)

// This synthetic profile permits staff classification; the statutory profiles do not.
var reviewableMarketingCountry = sync.OnceValue(func() string {
	messagingrules.Register(messagingrules.Rules{Jurisdiction: "zi", Version: 1})
	return "zi"
})

func TestMarketingNeedsConsentOrASupportedStatutoryRoute(t *testing.T) {
	if _, registered := messagingrules.For("zz"); registered {
		t.Fatal("the unsupported-country fixture has a messaging profile")
	}
	for _, country := range []string{"", "zz", customerRoute()} {
		for _, standing := range []bool{false, true} {
			t.Run(country+map[bool]string{false: "/normal", true: "/standing"}[standing], func(t *testing.T) {
				e := setupResolve(t)
				e.seedPurpose(t, "newsletter", "marketing")
				if country != "" {
					e.livesIn(t, country)
				}
				if standing {
					e.seedOverride(t, "marketing", false)
				}
				for _, req := range []commsauthz.Request{
					{LegacyPurposeKey: "newsletter"},
					{Context: commsauthz.CategoryMarketing},
				} {
					d := e.decide(t, req)
					if d.Verdict != commsauthz.VerdictDeny || d.ReasonCode != commsauthz.ReasonMarketingPermissionRequired {
						t.Fatalf("marketing without a supported basis: %s %s", d.Verdict, d.ReasonCode)
					}
					set := commsauthz.DecisionSet{Decisions: []commsauthz.Decision{d}}
					if d.CanBeOverruledByCategory() || set.ExceptionMayAnswer(false) || set.ExceptionMayAnswer(true) {
						t.Fatal("a staff instruction replaced the required marketing basis")
					}
				}
			})
		}
	}
}

func TestCategoryOnlyLeadMarketingKeepsItsBoundaryAtStagingAndDispatch(t *testing.T) {
	e := setupLeadConsent(t)
	gate := NewGate(e.store)
	delivery := plantConsentDelivery(t, e.owner, e.user)
	recipients := []connector.Recipient{{Email: e.leadEmail}}
	err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		set, err := gate.AuthorizeStagingTx(e.ctx, tx, delivery, commsauthz.Request{
			Recipients: recipients, Context: commsauthz.CategoryMarketing, Subject: "S", Body: "b",
		})
		if err != nil {
			return err
		}
		if len(set.Decisions) != 1 {
			t.Fatalf("staging checked %d recipients", len(set.Decisions))
		}
		decision := set.Decisions[0]
		if decision.Requested != commsauthz.CategoryMarketing || decision.ReasonCode != commsauthz.ReasonMarketingPermissionRequired {
			t.Fatalf("staging lost the lead's marketing claim: %+v", decision)
		}
		if set.ExceptionMayAnswer(true) {
			t.Fatal("staging offered a staff exception")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := gate.AuthorizeTransmit(e.ctx, commsauthz.TransmitRequest{
		DeliveryID: delivery, Attempt: 1, Recipients: recipients, Subject: "S", Body: "b",
	})
	if err != nil {
		t.Fatal(err)
	}
	if ticket.Allowed || ticket.ConsentRefused {
		t.Fatal("dispatch offered an exception for the lead's unsupported marketing")
	}
	var requested, reason string
	if err := e.owner.QueryRow(context.Background(), `SELECT requested_category, reason_code FROM communication_decision
 WHERE delivery_id=$1 AND phase='transmit'`, delivery).Scan(&requested, &reason); err != nil {
		t.Fatal(err)
	}
	if requested != string(commsauthz.CategoryMarketing) || reason != commsauthz.ReasonMarketingPermissionRequired {
		t.Fatalf("dispatch recorded %q / %q", requested, reason)
	}
}

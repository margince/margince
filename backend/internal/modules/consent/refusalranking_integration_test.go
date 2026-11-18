// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// A message whose To lacks its footer and whose Bcc objected names the
// objection first, in the review and in the 409 alike, whatever order the
// recipients were written in.
func TestAMixedRefusalNamesTheObjectionFirst(t *testing.T) {
	e := setupResolve(t)
	e.seedPurpose(t, "newsletter", "marketing")
	e.suppress(t, "marketing_objection")
	to := "kai@buyer.test"
	e.secondContactWhoWroteAMonthAgo(t, to)
	delivery := e.plantDelivery(t)

	var set commsauthz.DecisionSet
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		set, err = e.gate.AuthorizeStagingTx(e.ctx, tx, delivery, commsauthz.Request{
			Recipients:       []connector.Recipient{{Email: to}, {Email: e.address}},
			ThreadKey:        "kai-question",
			LegacyPurposeKey: "newsletter",
			Evidence:         commsauthz.Evidence{FooterFindings: []commsauthz.FooterFinding{{Kind: "objection_route"}}},
			Subject:          "S", Body: "b",
		})
		return err
	}); err != nil {
		t.Fatalf("staging: %v", err)
	}
	if first := set.Decisions[0]; first.ReasonCode != commsauthz.ReasonFooterIncomplete {
		t.Fatalf("the To decided %s %q, want the footer finding: the fixture proves nothing", first.Verdict, first.ReasonCode)
	}

	review, err := e.gate.RecordRefusal(e.ctx, set, ids.UUID{})
	if err != nil {
		t.Fatalf("recording the refusal: %v", err)
	}
	if review.ReasonCode != commsauthz.ReasonObjection || review.Refusals[0].Address != e.address {
		t.Errorf("the review names %q for %s, want the objection of %s", review.ReasonCode, review.Refusals[0].Address, e.address)
	}
	if blocking := BlockingRefusals(set); blocking[0].ReasonCode != commsauthz.ReasonObjection {
		t.Errorf("the 409 names %q first, want the objection", blocking[0].ReasonCode)
	}
}

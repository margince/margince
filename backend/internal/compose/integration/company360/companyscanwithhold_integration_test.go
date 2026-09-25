// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package company360

// A stored finding outlives the GRANTS it was written under, not only the
// records it rests on.
//
// Retraction answers the existence question and is tested with the lifecycle.
// This is the content one: the scan quotes a message's subject and a verbatim
// extract of its body into the row, and an audience narrowed afterwards has to
// reach those words on every replay. Over a real database because the whole
// question is a SQL predicate — the audience arm admitting this reader or not —
// and a fake reader would only ever answer whatever it was told to.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/companyscan"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestNarrowingAMessagesAudienceTakesItsWordsOffTheStoredFinding(t *testing.T) {
	e := integration.Setup(t)
	company := ids.From[ids.CompanyKind](e.SeedCompany(t, "Nordlicht", &e.Rep1))
	message := seedInboundAsk(t, e, company.UUID, "workspace")
	const quoted = "wants to see a sample of the driver reports"
	lane := &quotingLane{messageID: message.String(), quote: quoted}
	var queued []companyscan.Queued
	svc := scanClocked(e, lane, &queued, time.Now)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AccountRepPerms)

	done := readOnce(rep, t, svc, company, &queued)
	cited := quotedCitation(t, done)
	if cited.Name == nil || cited.Quote == nil {
		t.Fatalf("the scan stored a finding with no words to withhold: name %v, quote %v — "+
			"this suite proves they are taken away, so it has to see them arrive", cited.Name, cited.Quote)
	}

	// The narrowing. captured_by on the fixture is `human:x` and this reader is
	// on no participant row, so the content arm stops admitting them — while the
	// company link keeps the row discoverable, which is the whole reason this is
	// not a retraction.
	e.WsExec(t, `UPDATE activity SET audience = 'participants' WHERE id = $1`, message)

	after, err := svc.Get(rep, company)
	if err != nil {
		t.Fatalf("get after the narrowing: %v", err)
	}
	withheld := quotedCitation(t, after)
	if withheld.Name != nil {
		t.Errorf("the replayed finding still names the message %q after its audience excluded this reader", *withheld.Name)
	}
	if withheld.Quote != nil {
		t.Errorf("the replayed finding still quotes %q after its audience excluded this reader", *withheld.Quote)
	}
	// And the advice itself is kept. The reader may still know the message is
	// there, so dropping the card would lose guidance they are entitled to.
	if withheld.EmailSummary != nil {
		t.Errorf("the citation was given a live summary the reader may no longer receive")
	}
}

// quotedCitation is the one activity citation the scan's finding rests on.
func quotedCitation(t *testing.T, scan crmcontracts.CompanyScan) crmcontracts.CompanyBriefEvidence {
	t.Helper()
	for _, finding := range scan.Findings {
		for _, evidence := range finding.Evidence {
			if evidence.EntityType == crmcontracts.CompanyBriefEvidenceEntityTypeActivity {
				return evidence
			}
		}
	}
	t.Fatalf("no finding citing an activity in a scan of %d findings; the read this suite needs did not happen", len(scan.Findings))
	return crmcontracts.CompanyBriefEvidence{}
}

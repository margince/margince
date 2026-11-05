// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Stage evidence says whether the reader can open its source, through the
// real wiring: the activity and contract readers' own gates, not a copy.
// A source the reader may not see reads like a deleted one.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// evidenceReaderPerms reads the deal and both kinds of source.
var evidenceReaderPerms = principal.Permissions{
	RoleKeys: []string{roleRep},
	Objects: map[string]principal.ObjectGrant{
		objContact: {Read: true},
		objCompany: {Read: true},
		objDeal:    {Create: true, Read: true, Update: true},
		"activity": {Create: true, Read: true, Update: true},
		"contract": {Read: true},
	},
	RowScope: principal.RowScopeTeam,
}

type evidenceSources struct {
	deal               ids.DealID
	activity, contract ids.UUID
	// onArchivedDeal is a contract whose deal was archived, which the
	// contract reader hides from everyone.
	onArchivedDeal ids.UUID
}

// seedEvidenceSources writes a deal Rep1 owns with three claims on one
// criterion. One cites a mail Rep1 limited to its participants. One cites a
// contract on the deal, and one a contract on a second, archived deal.
func seedEvidenceSources(t *testing.T, e *Env) evidenceSources {
	t.Helper()
	admin := e.Admin()
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, evidenceReaderPerms)
	pipeline, open, _ := DealFixture(t, e)
	dealUUID := e.SeedDeal(t, "Evidence sources", pipeline, open, &e.Rep1)
	deal := ids.From[ids.DealKind](dealUUID)
	criterion, err := e.Deals.CreateStageExitCriterion(admin, deals.CreateCriterionInput{
		StageID: open, Key: "problem", Label: "The buyer confirmed the problem",
		Kind: string(deals.CriterionBuyerConfirmed),
	})
	if err != nil {
		t.Fatalf("configuring the criterion: %v", err)
	}

	subject, body := "Re: the problem", "Yes, that is exactly our problem."
	mail, _, err := e.Activities.LogActivity(author, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: new("inbound"),
		Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: dealUUID}},
	})
	if err != nil {
		t.Fatalf("logging the mail: %v", err)
	}
	mailID := ids.From[ids.ActivityKind](ids.UUID(mail.Id))
	if _, err := e.Activities.SetAudience(author, mailID,
		activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatalf("limiting the mail to its participants: %v", err)
	}
	contractID := ids.UUID(seedContract(t, e, contracts.CreateContractInput{
		CompanyID: companyIDOf(e.SeedCompany(t, "Evidence Co", nil)), DealID: &deal,
		Title: "The agreement", ValueBasis: contracts.BasisTotal, Source: "manual",
	}).UUID)

	closedDeal := ids.From[ids.DealKind](e.SeedDeal(t, "An archived deal", pipeline, open, &e.Rep1))
	onArchivedDeal := ids.UUID(seedContract(t, e, contracts.CreateContractInput{
		CompanyID: companyIDOf(e.SeedCompany(t, "Archived Deal Co", nil)), DealID: &closedDeal,
		Title: "The old agreement", ValueBasis: contracts.BasisTotal, Source: "manual",
	}).UUID)
	if _, err := e.Deals.ArchiveDeal(admin, closedDeal, nil); err != nil {
		t.Fatalf("archiving the second deal: %v", err)
	}

	for _, source := range []struct {
		kind string
		id   ids.UUID
	}{{deals.SourceActivity, mailID.UUID}, {deals.SourceContract, contractID}, {deals.SourceContract, onArchivedDeal}} {
		if _, err := e.Deals.RecordStageEvidence(admin, deals.EvidenceInput{
			DealID: deal, CriterionID: ids.From[ids.ExitCriterionKind](ids.UUID(criterion.Id)),
			SourceType: source.kind, SourceID: source.id, AuthorSide: deals.AuthorBuyer,
			Commitment: deals.CommitmentAgreed, Met: true,
			ObservedAt: time.Now().UTC(), ExtractedBy: deals.ExtractedByDeterministic,
		}); err != nil {
			t.Fatalf("recording evidence read from the %s: %v", source.kind, err)
		}
	}
	return evidenceSources{deal: deal, activity: mailID.UUID, contract: contractID, onArchivedDeal: onArchivedDeal}
}

// missingBySource lists the deal's evidence as reader and answers
// source_missing per source id.
func missingBySource(t *testing.T, e *Env, reader principal.Permissions, user ids.UUID, deal ids.DealID) map[ids.UUID]bool {
	t.Helper()
	rows, err := e.Deals.ListStageEvidence(e.As(user, []ids.UUID{e.Team1}, reader), deal)
	if err != nil {
		t.Fatalf("listing the evidence: %v", err)
	}
	out := map[ids.UUID]bool{}
	for _, row := range rows {
		out[ids.UUID(row.SourceId)] = row.SourceMissing
	}
	return out
}

func TestEvidenceShowsASourceTheReaderCannotOpenAsMissing(t *testing.T) {
	e := Setup(t)
	src := seedEvidenceSources(t, e)

	owner := missingBySource(t, e, evidenceReaderPerms, e.Rep1, src.deal)
	if owner[src.activity] || owner[src.contract] {
		t.Fatalf("the mail's author sees %v, want the mail and the live contract openable", owner)
	}
	if !owner[src.onArchivedDeal] {
		t.Error("a contract the contract reader hides reads as openable, so the two readers disagree")
	}

	// Rep2 shares Rep1's team, so the deal and its contract are visible, but
	// the mail is limited to its participants.
	teammate := missingBySource(t, e, evidenceReaderPerms, e.Rep2, src.deal)
	if !teammate[src.activity] {
		t.Error("a teammate outside the mail's audience sees it as openable, so source_missing tells them a limited mail exists")
	}
	if teammate[src.contract] {
		t.Error("a teammate who may read the deal's contract sees it as missing")
	}

	// ContractRepPerms reads contracts but not activities, so the mail's own
	// author cannot open it under that role.
	authorWithoutActivityRead := missingBySource(t, e, ContractRepPerms, e.Rep1, src.deal)
	if !authorWithoutActivityRead[src.activity] || authorWithoutActivityRead[src.contract] {
		t.Errorf("a reader without activity read sees %v, want only the mail missing", authorWithoutActivityRead)
	}

	// RepPerms reads deals but neither activities nor contracts.
	withoutReads := missingBySource(t, e, RepPerms, e.Rep2, src.deal)
	if !withoutReads[src.activity] || !withoutReads[src.contract] {
		t.Errorf("a reader without activity or contract read sees %v, want both missing", withoutReads)
	}
}

// The source pair has no foreign key, so a hard delete of a cited activity
// goes through. No product path hard-deletes one, so the delete is SQL.
func TestEvidenceOutlivesADeletedActivity(t *testing.T) {
	e := Setup(t)
	src := seedEvidenceSources(t, e)
	e.WsExec(t, `DELETE FROM activity WHERE id = $1`, src.activity)

	owner := missingBySource(t, e, evidenceReaderPerms, e.Rep1, src.deal)
	if len(owner) != 3 {
		t.Fatalf("the ledger holds %d sources after the delete, want every observation kept", len(owner))
	}
	if !owner[src.activity] {
		t.Error("the deleted mail reads as openable, so a client follows source_id to nothing")
	}
	if owner[src.contract] {
		t.Error("deleting the mail marked the contract missing too")
	}

	var facts deals.StageProgressionFacts
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, evidenceReaderPerms)
	if err := e.Deals.Tx(reader, func(tx pgx.Tx) error {
		var err error
		facts, err = e.Deals.ReadStageProgressionFacts(reader, tx, src.deal, deals.AutopilotFacts{}, time.Now())
		return err
	}); err != nil {
		t.Fatalf("the progression facts stopped reading once a source was deleted: %v", err)
	}
	if !facts.Criteria[0].Met {
		t.Error("the criterion lost its settlement when one source was deleted")
	}
	for _, cited := range facts.Criteria[0].Sources {
		if cited.SourceID == src.activity && !cited.Missing {
			t.Error("the progression facts cite the deleted mail as openable")
		}
		if cited.SourceID == src.contract && cited.Missing {
			t.Error("the progression facts mark the live contract missing")
		}
	}
}

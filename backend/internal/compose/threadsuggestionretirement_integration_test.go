// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestARepairDoesNotRetireATagSuggestionWhoseEvidenceWillReturn(t *testing.T) {
	e := setupTagScout(t)
	tag := e.tag(t, "Product X", "pricing for Product X", true)
	contact := e.SeedContact(t, "Pat Buyer", &e.Rep1)
	at := clocktest.Now(t).Truncate(time.Microsecond)
	subject, body := "Product X", "Please send pricing for Product X."
	key := "tag-evidence@buyer.example"
	source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "email", Subject: &subject, Body: &body, Direction: new("inbound"), OccurredAt: &at, Source: "manual", ThreadKey: key, Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}})
	if err != nil {
		t.Fatal(err)
	}
	system := principal.SystemActing(e.Admin(), "system:test_tag_retirement")
	if err := database.WithWorkspaceTx(system, e.Pool, func(tx pgx.Tx) error {
		raised, err := collections.RecordTagSuggestionTx(system, tx, collections.TagSuggestionDraft{TagID: tag, EntityType: "contact", EntityID: contact, Evidence: []collections.TagSuggestionEvidence{{ActivityID: ids.UUID(source.Id), OccurredAt: at}}})
		if err == nil && !raised {
			t.Fatal("valid tag suggestion was not recorded")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	id := ids.MustParse(e.WsScalar(t, `SELECT id::text FROM tag_suggestion WHERE tag_id=$1`, tag))
	retire := func() int {
		t.Helper()
		count := 0
		if err := database.WithWorkspaceTx(system, e.Pool, func(tx pgx.Tx) error {
			var err error
			count, err = collections.SupersedeStaleTagSuggestionsTx(system, tx)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return count
	}
	run := beginUnavailableThread(t, e.Env, key)
	if n := retire(); n != 0 {
		t.Fatalf("repair retired%d tag suggestions", n)
	}
	if state := e.WsScalar(t, `SELECT state FROM tag_suggestion WHERE id=$1`, id); state != "open" {
		t.Fatalf("pending suggestion state=%s", state)
	}
	if _, err := e.tags.GetTagSuggestion(e.Admin(), id); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("hidden evidence suggestion=%v", err)
	}
	finishUnavailableThread(t, e.Env, run)
	if n := retire(); n != 0 {
		t.Fatalf("publication retired%d tag suggestions", n)
	}
	shown, err := e.tags.GetTagSuggestion(e.Admin(), id)
	if err != nil || shown.ID != id {
		t.Fatalf("published same suggestion=%+v %v", shown, err)
	}
	if _, err := e.Activities.ArchiveActivity(e.Admin(), ids.From[ids.ActivityKind](ids.UUID(source.Id)), nil); err != nil {
		t.Fatal(err)
	}
	if n := retire(); n != 1 {
		t.Fatalf("archived source retired%d suggestions", n)
	}
	if state := e.WsScalar(t, `SELECT state FROM tag_suggestion WHERE id=$1`, id); state != "superseded" {
		t.Fatalf("archived evidence state=%s", state)
	}
}

func TestARepairDoesNotRetireADealSuggestionFromAnyEvidenceKind(t *testing.T) {
	for _, kind := range []string{deals.EvidenceMeeting, deals.EvidenceSignal, deals.EvidenceAttachment} {
		t.Run(kind, func(t *testing.T) {
			e := setupScout(t)
			e.now = clocktest.Now(t).Truncate(time.Microsecond)
			company := e.SeedCompany(t, "Buyer Company", nil)
			key := "suggestion-evidence@buyer.example"
			buyerThread := ""
			if kind == deals.EvidenceSignal {
				buyerThread = key
			}
			source, buyer := e.buyingEmail(t, company, e.daysAgo(4), buyerThread)
			signal := e.signal(t, "new_opportunity", company, source, e.daysAgo(4), ids.Nil, buyer)
			evidence := []deals.SuggestionEvidence{{Kind: deals.EvidenceSignal, SignalID: &signal, OccurredAt: e.daysAgo(4)}}
			if kind != deals.EvidenceSignal {
				if kind == deals.EvidenceMeeting {
					source = e.meeting(e.Admin(), t, "Scoping workshop", &buyer, e.daysAgo(2))
				} else {
					source = e.email(t, "Proposal", "outbound", company, e.daysAgo(2))
				}
				e.WsExec(t, `UPDATE activity SET thread_key=$2 WHERE id=$1`, source, key)
				item := deals.SuggestionEvidence{Kind: kind, OccurredAt: e.daysAgo(2)}
				if kind == deals.EvidenceMeeting {
					item.ActivityID = &source
				} else {
					attachment := e.document(t, source, "Proposal.pdf", "application/pdf")
					item.AttachmentID = &attachment
				}
				evidence = append(evidence, item)
			}
			raised, err := e.record(e.system(), t, deals.SuggestionDraft{CompanyID: company, NameHint: deals.HintOpportunitySignaled, Confidence: 0.9, Evidence: evidence})
			if err != nil || !raised {
				t.Fatalf("recording suggestion=%v %v", raised, err)
			}
			id := e.onlySuggestion(e.signalReader(), t, company).ID
			run := beginUnavailableThread(t, e.Env, key)
			if pass := e.pass(t); pass.Superseded != 0 {
				t.Fatalf("repair retired%d suggestions", pass.Superseded)
			}
			if e.stored(t, company, deals.SuggestionOpen) != 1 {
				t.Fatal("hidden evidence permanently retired its suggestion")
			}
			if shown := e.suggestions(e.signalReader(), t, company); len(shown) != 0 {
				t.Fatal("hidden evidence was published")
			}
			finishUnavailableThread(t, e.Env, run)
			if pass := e.pass(t); pass.Superseded != 0 {
				t.Fatalf("publication retired%d suggestions", pass.Superseded)
			}
			if shown := e.onlySuggestion(e.signalReader(), t, company); shown.ID != id {
				t.Fatal("publication replaced the suggestion instead of preserving it")
			}
			if _, err := e.Activities.ArchiveActivity(e.Admin(), ids.From[ids.ActivityKind](source), nil); err != nil {
				t.Fatal(err)
			}
			if pass := e.pass(t); pass.Superseded != 1 {
				t.Fatalf("archived source retired%d suggestions", pass.Superseded)
			}
			if state := e.WsScalar(t, `SELECT state FROM deal_suggestion WHERE id=$1`, id); state != deals.SuggestionSuperseded {
				t.Fatalf("archived evidence state=%s", state)
			}
		})
	}
}

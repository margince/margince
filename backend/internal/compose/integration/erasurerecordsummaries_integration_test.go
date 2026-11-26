// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A deal's status card and an account brief are prose written from the record's
// timeline, held per reader and keyed by the record. Neither privacy act removes
// the deal or the company, so only a named purge reaches them.

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/generatedsummary"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func TestBothPrivacyActsDestroyTheSummariesWrittenFromTheSubjectsWords(t *testing.T) {
	acts := map[string]func(*Env, ids.UUID) error{
		"erasure": func(e *Env, subject ids.UUID) error {
			return privacy.NewEraser(e.DB()).WithEdgeInvalidator(search.RecomputeEdgesForActivities).EraseContact(e.Admin(), subject, "subject request")
		},
		"anonymise": func(e *Env, subject ids.UUID) error {
			service := privacy.NewRetentionService(e.DB(), nil, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
			_, err := service.AnonymiseContacts(e.Admin(), []ids.UUID{subject}, privacy.PurgeOwnerRule,
				func(context.Context, pgx.Tx, ids.UUID) (bool, error) { return true, nil })
			return err
		},
	}
	for name, act := range acts {
		t.Run(name, func(t *testing.T) {
			e := Setup(t)
			subject := e.SeedContact(t, "Quoted Subject", &e.AdminUser)
			pipeline, open, _ := DealFixture(t, e)
			seed := func(account string, speaker ids.UUID) summarisedRecord {
				company := e.SeedCompany(t, account, &e.AdminUser)
				return summarise(t, e, speaker, company, rolloutOn(t, e, company, pipeline, open))
			}
			spokenOn := seed("Subject Account", subject)
			untouched := seed("Bystander Account", e.SeedContact(t, "Unrelated Contact", &e.AdminUser))

			if err := act(e, subject); err != nil {
				t.Fatalf("the %s → %v", name, err)
			}
			if n := spokenOn.cached(t, e); n != 0 {
				t.Errorf("%d summaries of the deal and account the subject wrote to survive the %s, "+
					"each holding prose written from their words", n, name)
			}
			if n := untouched.cached(t, e); n != 2 {
				t.Errorf("the bystander's deal and account hold %d summaries, want 2: a purge of "+
					"every cached summary would read as a pass on the assertion above", n)
			}
		})
	}
}

// summarisedRecord is a deal on an account, each with one reader's summary cached.
type summarisedRecord struct{ company, deal ids.UUID }

func rolloutOn(t *testing.T, e *Env, company ids.UUID, pipeline ids.PipelineID, open ids.StageID) ids.UUID {
	t.Helper()
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Source: provenance.RecordSourceManual, Name: "Rollout", PipelineID: pipeline, StageID: open,
		CompanyID: new(ids.From[ids.CompanyKind](company)),
	})
	if err != nil {
		t.Fatalf("opening a deal on the account: %v", err)
	}
	return ids.UUID(deal.Id)
}

// summarise files a note from the speaker under the deal alone, then caches a
// status card and an account brief.
//
// The note is not filed under the account, so the brief is reached through the deal.
func summarise(t *testing.T, e *Env, speaker, company, deal ids.UUID) summarisedRecord {
	t.Helper()
	record := summarisedRecord{company: company, deal: deal}
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "note", Subject: new("Retrofit timeline"), Body: new("We need it before March."), Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: speaker}, {EntityType: "deal", EntityID: deal}},
	}); err != nil {
		t.Fatalf("filing the speaker's note under the deal: %v", err)
	}
	reader := ids.From[ids.UserKind](e.AdminUser)
	for _, subject := range []generatedsummary.Subject{
		generatedsummary.DealStatusCard(ids.From[ids.DealKind](record.deal)),
		generatedsummary.CompanyBrief(ids.From[ids.CompanyKind](record.company)),
	} {
		if err := generatedsummary.Save(e.Admin(), e.Pool, reader, subject, generatedsummary.Entry{
			Fingerprint: "fp-1", GeneratedAt: clocktest.Now(t), GeneratedBy: "deterministic", Payload: []byte(`{"headline":"Retrofit before March"}`),
		}); err != nil {
			t.Fatalf("caching a summary: %v", err)
		}
	}
	if n := record.cached(t, e); n != 2 {
		t.Fatalf("the fixture cached %d summaries, want 2", n)
	}
	return record
}

func (r summarisedRecord) cached(t *testing.T, e *Env) int {
	t.Helper()
	var n int
	if err := e.Pool.QueryRow(e.Admin(),
		`SELECT count(*) FROM generated_summary WHERE company_id = $1 OR deal_id = $2`, r.company, r.deal).Scan(&n); err != nil {
		t.Fatalf("counting the cached summaries: %v", err)
	}
	return n
}

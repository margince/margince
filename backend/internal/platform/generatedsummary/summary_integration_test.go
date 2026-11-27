// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package generatedsummary_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/generatedsummary"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var writtenAt = time.Date(2026, 5, 4, 9, 30, 0, 0, time.UTC)

// A summary is read back by the reader, kind and record it was stored under.
// A neighbour that differs in one of the three reads its own.
func TestASummaryIsKeyedOnItsReaderItsKindAndItsRecord(t *testing.T) {
	t.Parallel()
	e := setup(t)
	company, other := e.company(t), e.company(t)
	reader, colleague := e.reader(t), e.reader(t)
	brief := generatedsummary.CompanyBrief(company)

	e.store(t, reader, brief, "brief-v1")
	e.store(t, reader, generatedsummary.CompanyDossier(company), "dossier")
	e.store(t, colleague, brief, "the colleague's brief")
	e.store(t, reader, generatedsummary.CompanyBrief(other), "another company")
	e.store(t, reader, brief, "brief-v2")

	for _, c := range []struct {
		name    string
		reader  ids.UserID
		subject generatedsummary.Subject
		want    string
	}{
		{"the brief, replaced by its second write", reader, brief, "brief-v2"},
		{"the dossier of the same company", reader, generatedsummary.CompanyDossier(company), "dossier"},
		{"the colleague's brief", colleague, brief, "the colleague's brief"},
		{"the brief of another company", reader, generatedsummary.CompanyBrief(other), "another company"},
	} {
		got, found, err := generatedsummary.Read(e.ctx, e.pool, c.reader, c.subject)
		if err != nil || !found || got.Fingerprint != c.want {
			t.Errorf("%s read as (%q, %t, %v), want %q", c.name, got.Fingerprint, found, err, c.want)
		}
		if found && (!got.GeneratedAt.Equal(writtenAt) || got.GeneratedBy != "model" || string(got.Payload) != `{"n": 1}`) {
			t.Errorf("%s came back as %+v, which is not what was stored", c.name, got)
		}
	}
	if _, found, err := generatedsummary.Read(e.ctx, e.pool, reader,
		generatedsummary.CompanyGrowthFit(company)); err != nil || found {
		t.Errorf("a kind nobody stored read as found=%t, err=%v", found, err)
	}
	if n := e.count(t, `SELECT count(*) FROM generated_summary WHERE company_id = $1`, company); n != 3 {
		t.Errorf("the company holds %d summaries, want 3: a second write of one key replaces the first", n)
	}
}

// Each kind is stored against the record type it summarises and read by it.
func TestEachKindReadsBackFromItsOwnRecord(t *testing.T) {
	t.Parallel()
	e := setup(t)
	reader := e.reader(t)
	contact, deal, quiet := e.contact(t), e.deal(t), e.deal(t)
	e.store(t, reader, generatedsummary.CompanyGrowthFit(e.company(t)), "fit")
	e.store(t, reader, generatedsummary.ContactBrief(contact), "contact")
	e.store(t, reader, generatedsummary.DealStatusCard(deal), "card")

	if got, found, err := generatedsummary.Read(e.ctx, e.pool, reader,
		generatedsummary.ContactBrief(contact)); err != nil || !found || got.Fingerprint != "contact" {
		t.Errorf("the contact brief read as (%q, %t, %v)", got.Fingerprint, found, err)
	}
	var cards map[ids.UUID]generatedsummary.Entry
	if err := database.WithWorkspaceTx(e.ctx, e.pool, func(tx pgx.Tx) error {
		var err error
		cards, err = generatedsummary.ReadEachTx(e.ctx, tx, reader, generatedsummary.KindDealStatusCard,
			[]ids.UUID{deal.UUID, quiet.UUID, contact.UUID})
		return err
	}); err != nil {
		t.Fatalf("reading the cards: %v", err)
	}
	if len(cards) != 1 || cards[deal.UUID].Fingerprint != "card" {
		t.Errorf("the cards read as %+v, want the one stored deal and neither the deal without a card "+
			"nor the contact, whose id is not a deal's", cards)
	}
}

// A company merge moves a reader's summaries to the survivor, and keeps the
// survivor's own where the reader holds the same kind on both.
func TestACompanyMergeCarriesSummariesTheSurvivorLacks(t *testing.T) {
	t.Parallel()
	e := setup(t)
	reader, colleague := e.reader(t), e.reader(t)
	retired, survivor, bystander := e.company(t), e.company(t), e.company(t)
	e.store(t, colleague, generatedsummary.CompanyBrief(retired), "the colleague's retired brief")
	e.store(t, colleague, generatedsummary.CompanyDossier(survivor), "the colleague's survivor dossier")
	e.store(t, reader, generatedsummary.CompanyBrief(retired), "retired brief")
	e.store(t, reader, generatedsummary.CompanyDossier(retired), "retired dossier")
	e.store(t, reader, generatedsummary.CompanyBrief(survivor), "survivor brief")
	e.store(t, reader, generatedsummary.CompanyBrief(bystander), "bystander brief")

	e.inTx(t, func(tx pgx.Tx) error { return generatedsummary.CarryCompany(e.ctx, tx, retired, survivor) })

	for _, c := range []struct {
		reader  ids.UserID
		subject generatedsummary.Subject
		want    string
	}{
		{reader, generatedsummary.CompanyBrief(survivor), "survivor brief"},
		{reader, generatedsummary.CompanyDossier(survivor), "retired dossier"},
		{reader, generatedsummary.CompanyBrief(bystander), "bystander brief"},
		// Another reader's row on the survivor blocks neither reader's carry.
		{colleague, generatedsummary.CompanyBrief(survivor), "the colleague's retired brief"},
		{colleague, generatedsummary.CompanyDossier(survivor), "the colleague's survivor dossier"},
	} {
		got, found, err := generatedsummary.Read(e.ctx, e.pool, c.reader, c.subject)
		if err != nil || !found || got.Fingerprint != c.want {
			t.Errorf("after the merge a summary read as (%q, %t, %v), want %q", got.Fingerprint, found, err, c.want)
		}
	}
	if n := e.count(t, `SELECT count(*) FROM generated_summary WHERE company_id = $1`, retired); n != 0 {
		t.Errorf("%d summaries still name the merged-away company", n)
	}
}

// Forgetting a contact deletes what was written about it for all readers, and
// leaves the summaries of other records.
func TestForgettingAContactDeletesItsSummariesForAllReaders(t *testing.T) {
	t.Parallel()
	e := setup(t)
	reader, colleague := e.reader(t), e.reader(t)
	subject, second, bystander := e.contact(t), e.contact(t), e.contact(t)
	e.store(t, reader, generatedsummary.ContactBrief(subject), "subject")
	e.store(t, colleague, generatedsummary.ContactBrief(subject), "subject, for the colleague")
	e.store(t, reader, generatedsummary.ContactBrief(second), "second")
	e.store(t, reader, generatedsummary.ContactBrief(bystander), "bystander")

	e.inTx(t, func(tx pgx.Tx) error { return generatedsummary.ForgetContacts(e.ctx, tx, subject, second) })

	if n := e.count(t, `SELECT count(*) FROM generated_summary WHERE contact_id = ANY($1)`,
		[]ids.UUID{subject.UUID, second.UUID}); n != 0 {
		t.Errorf("%d summaries of the forgotten contacts remain", n)
	}
	if _, found, err := generatedsummary.Read(e.ctx, e.pool, reader,
		generatedsummary.ContactBrief(bystander)); err != nil || !found {
		t.Errorf("the bystander's brief read as found=%t, err=%v after another contact was forgotten", found, err)
	}
}

type env struct {
	ctx  context.Context
	pool *pgxpool.Pool
}

func setup(t *testing.T) env {
	t.Helper()
	dsn := testdb.For(t).OwnerDSN
	pool, err := testdb.OwnPool(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	return env{ctx: principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:summary-test",
	}), pool: pool}
}

func (e env) inTx(t *testing.T, fn func(pgx.Tx) error) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.ctx, e.pool, fn); err != nil {
		t.Fatal(err)
	}
}

func (e env) store(t *testing.T, reader ids.UserID, subject generatedsummary.Subject, fingerprint string) {
	t.Helper()
	if err := generatedsummary.Save(e.ctx, e.pool, reader, subject, generatedsummary.Entry{
		Fingerprint: fingerprint, GeneratedAt: writtenAt, GeneratedBy: "model", Payload: []byte(`{"n":1}`),
	}); err != nil {
		t.Fatalf("storing %q: %v", fingerprint, err)
	}
}

func (e env) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	e.inTx(t, func(tx pgx.Tx) error { return tx.QueryRow(e.ctx, query, args...).Scan(&n) })
	return n
}

func (e env) inserted(t *testing.T, query string, args ...any) ids.UUID {
	t.Helper()
	var id ids.UUID
	e.inTx(t, func(tx pgx.Tx) error { return tx.QueryRow(e.ctx, query, args...).Scan(&id) })
	return id
}

func (e env) reader(t *testing.T) ids.UserID {
	t.Helper()
	id := ids.NewV7()
	return ids.From[ids.UserKind](e.inserted(t, `
		INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, 'Reader') RETURNING id`,
		id, id.String()+"@summary.test"))
}

func (e env) company(t *testing.T) ids.CompanyID {
	t.Helper()
	return ids.From[ids.CompanyKind](e.inserted(t, `
		INSERT INTO company (display_name, source, captured_by)
		VALUES ('Summary Co', 'manual', 'human:summary-test') RETURNING id`))
}

func (e env) contact(t *testing.T) ids.ContactID {
	t.Helper()
	return ids.From[ids.ContactKind](e.inserted(t, `
		INSERT INTO contact (full_name, source, captured_by)
		VALUES ('Summary Contact', 'manual', 'human:summary-test') RETURNING id`))
}

func (e env) deal(t *testing.T) ids.DealID {
	t.Helper()
	name := "Summary " + ids.NewV7().String()
	return ids.From[ids.DealKind](e.inserted(t, `
		WITH p AS (
		  INSERT INTO pipeline (name) VALUES ($1) RETURNING id
		), s AS (
		  INSERT INTO stage (pipeline_id, name, position, semantic, win_probability)
		  SELECT id, 'Open', 1, 'open', 10 FROM p RETURNING id, pipeline_id
		)
		INSERT INTO deal (pipeline_id, stage_id, company_id, name, source, captured_by)
		SELECT s.pipeline_id, s.id, $2, 'Summary deal', 'manual', 'human:summary-test' FROM s RETURNING id`,
		name, e.company(t)))
}

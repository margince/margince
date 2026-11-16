// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package companyprofiles_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/companyprofiles"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestCompanyOfferCorrectionRunsThroughTheExecutor(t *testing.T) {
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:test")
	var idsToClear []ids.UUID
	for _, name := range []string{"Offer A", "Offer B"} {
		company, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: name, Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		id := ids.From[ids.CompanyKind](ids.UUID(company.Id))
		summary := "Supplies industrial bearings."
		if err := e.Contacts.ApplyDeepRead(ctx, contacts.DeepReadProposal{CompanyID: id, SourceURL: "https://example.test", SiteReadID: ids.NewV7(), Fields: []contacts.DeepReadField{{Field: "offer_summary", Value: summary, EvidenceSnippet: summary, SourceURL: "https://example.test", Confidence: 0.9}}}); err != nil {
			t.Fatal(err)
		}
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `UPDATE company SET description = $2 WHERE id = $1`, id, summary); err != nil {
				return err
			}
			return storekit.StampFields(ctx, tx, "company", id.UUID, "site_read", "agent:site_read", []storekit.FieldStamp{{Field: "description"}})
		}); err != nil {
			t.Fatal(err)
		}
		idsToClear = append(idsToClear, id.UUID)
	}
	task := companyprofiles.OfferDuplicatesTask()
	task.Steps[0].Limit = 1
	original := task.Steps[0].Batch
	rollback := errors.New("rollback after write")
	task.Steps[0].Batch = func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
		out, err := original(ctx, tx, in)
		if err != nil {
			return out, err
		}
		return out, rollback
	}
	now := clocktest.Now(t)
	engine, err := datatask.New(e.Pool, []datatask.Task{task}, "test", func() time.Time { return now }, func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	if count := e.WsCount(t, `SELECT count(*) FROM company WHERE id = ANY($1) AND description IS NOT NULL`, idsToClear); count != 2 {
		t.Fatal("failed batch committed a description clear")
	}
	task.Steps[0].Batch = original
	engine, err = datatask.New(e.Pool, []datatask.Task{task}, "test", func() time.Time { return now.Add(time.Hour) }, func(context.Context, pgx.Tx, datatask.Progress) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		if _, err := engine.Execute(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if count := e.WsCount(t, `SELECT count(*) FROM company WHERE id = ANY($1) AND description IS NOT NULL`, idsToClear); count != 0 {
		t.Fatalf("%d duplicated descriptions remain", count)
	}
	if _, err := task.Steps[0].Batch(ctx, nil, datatask.BatchIn{Cursor: []byte(`"invalid"`), Limit: 1}); err == nil {
		t.Fatal("malformed cursor accepted")
	}
}

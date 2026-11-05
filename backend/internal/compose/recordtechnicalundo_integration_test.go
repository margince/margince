// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/auditverb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// lookupObservedAt is the lookup's own clock, fixed so no assertion leans on
// when the test ran.
var lookupObservedAt = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func technicalUndoCompany(t *testing.T, e *integration.Env, name string) ids.CompanyID {
	t.Helper()
	company, err := e.Contacts.CreateCompany(e.Admin(), contacts.CreateCompanyInput{DisplayName: name, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	return ids.From[ids.CompanyKind](ids.UUID(company.Id))
}

// applyUndoLookup runs one DNS lookup through the real writer and answers its
// audit row.
func applyUndoLookup(t *testing.T, e *integration.Env, company ids.CompanyID, observations ...contacts.TechnicalObservation) ids.UUID {
	t.Helper()
	err := e.Contacts.ApplyTechnicalEnrichment(machineCtx(e), contacts.TechnicalEnrichment{
		CompanyID: company, Completed: []contacts.TechnicalLane{contacts.LaneDNS}, Observations: observations,
		ObservedAt: lookupObservedAt,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return latestAuditRowID(t, e, "company", company.UUID, string(auditverb.Update))
}

func undoObservation(field, key, value string) contacts.TechnicalObservation {
	return contacts.TechnicalObservation{Field: field, ValueKey: key, Value: value, Evidence: "Synthetic DNS record", SourceURL: "dns:technical.example"}
}

// technicalUndoFacts reads the company's fact rows, as stored.
func technicalUndoFacts(t *testing.T, e *integration.Env, company ids.CompanyID) []map[string]any {
	t.Helper()
	var facts []map[string]any
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(e.Admin(), `SELECT to_jsonb(f) FROM company_fact f WHERE company_id = $1 ORDER BY field, value_key`, company)
		if err != nil {
			return err
		}
		facts, err = pgx.CollectRows(rows, pgx.RowTo[map[string]any])
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return facts
}

// reversalWriteShape is the audit row that reverses auditID and the outbox
// event committed with it.
func reversalWriteShape(t *testing.T, e *integration.Env, auditID ids.UUID) (reversals, events int) {
	t.Helper()
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(e.Admin(), `
			SELECT count(DISTINCT a.id), count(o.id)
			  FROM audit_log a
			  LEFT JOIN event_outbox o ON (o.envelope->'trace'->>'audit_log_id')::uuid = a.id
			                          AND o.envelope->>'type' = 'company.updated'
			 WHERE a.evidence->>'undid_audit_log_id' = $1::text AND a.action = 'restore'`, auditID).Scan(&reversals, &events)
	})
	if err != nil {
		t.Fatal(err)
	}
	return reversals, events
}

func withoutVersioning(facts []map[string]any) []map[string]any {
	out := make([]map[string]any, 0, len(facts))
	for _, fact := range facts {
		copied := map[string]any{}
		for k, v := range fact {
			if k != "version" && k != "updated_at" {
				copied[k] = v
			}
		}
		out = append(out, copied)
	}
	return out
}

func sameFacts(t *testing.T, a, b []map[string]any) bool {
	t.Helper()
	left, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	right, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	return string(left) == string(right)
}

func TestTechnicalUndoPutsBackTheReplacedFactsOnceWithTheirSource(t *testing.T) {
	e := integration.Setup(t)
	company := technicalUndoCompany(t, e, "Lookup Undo GmbH")
	applyUndoLookup(t, e, company, undoObservation("mail_provider", "old", "Old mail"), undoObservation("hosting_provider", "host", "Old host"))
	before := technicalUndoFacts(t, e, company)
	receipt := applyUndoLookup(t, e, company, undoObservation("mail_provider", "new", "New mail"))

	if answer := advisoryAnswer(e.Admin(), t, e, "company", company.UUID, receipt); !answer.Undoable {
		t.Fatalf("the preview refused a lookup nothing has touched since: %+v", answer)
	}
	if err := undoEntry(t, e, "company", company.UUID, receipt); err != nil {
		t.Fatalf("undo: %v", err)
	}
	after := technicalUndoFacts(t, e, company)
	if !sameFacts(t, withoutVersioning(before), withoutVersioning(after)) {
		t.Fatalf("undo did not put back the replaced rows with their source:\nbefore %v\nafter  %v", before, after)
	}
	for i := range before {
		if after[i]["version"].(float64) <= before[i]["version"].(float64) {
			t.Errorf("fact %v came back at version %v, so an edit token from before the lookup matches it again",
				after[i]["field"], after[i]["version"])
		}
	}
	if reversals, events := reversalWriteShape(t, e, receipt); reversals != 1 || events != 1 {
		t.Fatalf("the undo committed %d reversal row(s) and %d company.updated event(s), want 1 and 1", reversals, events)
	}

	if reason := refusedFor(t, undoEntry(t, e, "company", company.UUID, receipt)); reason != ReasonAlreadyUndone {
		t.Fatalf("a second Undo answered %s, want %s", reason, ReasonAlreadyUndone)
	}
	if reversals, _ := reversalWriteShape(t, e, receipt); reversals != 1 {
		t.Fatalf("the repeated Undo wrote again: %d reversal rows", reversals)
	}
}

func TestTwoUndosOfOneLookupPutItBackOnce(t *testing.T) {
	e := integration.Setup(t)
	company := technicalUndoCompany(t, e, "Concurrent Undo GmbH")
	applyUndoLookup(t, e, company, undoObservation("mail_provider", "old", "Old mail"))
	receipt := applyUndoLookup(t, e, company, undoObservation("mail_provider", "new", "New mail"))
	version := currentVersion(t, e, "company", company.UUID)

	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Go(func() {
			_, errs[i] = restoreSeamFor(e).Restore(e.Admin(), "company", company.UUID, receipt, version)
		})
	}
	wg.Wait()
	succeeded := 0
	for _, err := range errs {
		if err == nil {
			succeeded++
			continue
		}
		var refusal RefusedRestore
		if !errors.As(err, &refusal) || refusal.Reason != ReasonAlreadyUndone {
			t.Fatalf("the losing Undo answered %v (%+v), want a refusal saying it was already undone", err, refusal)
		}
	}
	if succeeded != 1 {
		t.Fatalf("%d of two concurrent Undos succeeded, want exactly one", succeeded)
	}
	if reversals, events := reversalWriteShape(t, e, receipt); reversals != 1 || events != 1 {
		t.Fatalf("two Undos left %d reversal rows and %d events, want 1 and 1", reversals, events)
	}
}

// Every later write to the company or its facts refuses the Undo, and the
// refusal leaves the rows as they were.
func TestTechnicalUndoRefusesAfterALaterCompanyOrFactWrite(t *testing.T) {
	cases := map[string]func(t *testing.T, e *integration.Env, company ids.CompanyID){
		"an unrelated company edit": func(t *testing.T, e *integration.Env, company ids.CompanyID) {
			if _, err := e.Contacts.UpdateCompany(e.Admin(), company, contacts.UpdateCompanyInput{Description: new("A later note")}); err != nil {
				t.Fatal(err)
			}
		},
		"a refresh to the same value": func(t *testing.T, e *integration.Env, company ids.CompanyID) {
			applyUndoLookup(t, e, company, undoObservation("mail_provider", "new", "New mail"))
		},
		"a human confirming the fact": func(t *testing.T, e *integration.Env, company ids.CompanyID) {
			if _, err := e.Contacts.ConfirmCompanyFact(e.Admin(), company, "mail_provider:new", contacts.FactWriteInput{}); err != nil {
				t.Fatal(err)
			}
		},
		"a human creating and removing a fact (empty ABA)": func(t *testing.T, e *integration.Env, company ids.CompanyID) {
			fact, err := e.Contacts.CreateCompanyFact(e.Admin(), company, contacts.FactCreateInput{Category: "signal", Field: "hosting_provider", Value: "Hand host"})
			if err != nil {
				t.Fatal(err)
			}
			if err := e.Contacts.DeleteCompanyFact(e.Admin(), company, "hosting_provider:"+fact.ValueKey, contacts.FactWriteInput{}); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, later := range cases {
		t.Run(name, func(t *testing.T) {
			e := integration.Setup(t)
			company := technicalUndoCompany(t, e, "Later Write GmbH")
			applyUndoLookup(t, e, company, undoObservation("mail_provider", "old", "Old mail"), undoObservation("hosting_provider", "host", "Old host"))
			receipt := applyUndoLookup(t, e, company, undoObservation("mail_provider", "new", "New mail"))
			later(t, e, company)
			held := technicalUndoFacts(t, e, company)

			if answer := advisoryAnswer(e.Admin(), t, e, "company", company.UUID, receipt); answer.Undoable ||
				answer.Reason != string(ReasonLaterCompanyChange) {
				t.Fatalf("the preview answered %+v, want a refusal naming the later change", answer)
			}
			if reason := refusedFor(t, undoEntry(t, e, "company", company.UUID, receipt)); reason != ReasonLaterCompanyChange {
				t.Fatalf("the Undo answered %s, want %s", reason, ReasonLaterCompanyChange)
			}
			if !sameFacts(t, held, technicalUndoFacts(t, e, company)) {
				t.Fatal("a refused Undo changed the facts")
			}
			if reversals, events := reversalWriteShape(t, e, receipt); reversals != 0 || events != 0 {
				t.Fatalf("a refused Undo committed %d reversal rows and %d events", reversals, events)
			}
		})
	}
}

// A refresh that observed what the company already held writes its audit row
// and nothing else. It has no event and no snapshot to offer back.
func TestASameValueRefreshIsAnObservationAndNotAChange(t *testing.T) {
	e := integration.Setup(t)
	company := technicalUndoCompany(t, e, "Refresh GmbH")
	applyUndoLookup(t, e, company, undoObservation("mail_provider", "mail", "Mail"))
	refresh := applyUndoLookup(t, e, company, undoObservation("mail_provider", "mail", "Mail"))
	var changes float64
	var snapshot, events int
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(e.Admin(), `
			SELECT (a.evidence->'technical'->>'changes')::float,
			       (a.evidence->'technical' ? 'restore')::int,
			       (SELECT count(*) FROM event_outbox o WHERE (o.envelope->'trace'->>'audit_log_id')::uuid = a.id)
			  FROM audit_log a WHERE a.id = $1`, refresh).Scan(&changes, &snapshot, &events)
	})
	if err != nil {
		t.Fatal(err)
	}
	if changes != 0 || snapshot != 0 || events != 0 {
		t.Fatalf("the refresh recorded changes=%v snapshot=%d events=%d, want 0, 0, 0", changes, snapshot, events)
	}
	if answer := advisoryAnswer(e.Admin(), t, e, "company", company.UUID, refresh); answer.Undoable {
		t.Fatal("a refresh that changed nothing was offered as undoable")
	}
}

func TestTechnicalUndoRefusesAnArchivedOrMergedCompany(t *testing.T) {
	e := integration.Setup(t)
	archived := technicalUndoCompany(t, e, "Archived Lookup GmbH")
	receipt := applyUndoLookup(t, e, archived, undoObservation("mail_provider", "mail", "Mail"))
	if _, err := e.Contacts.ArchiveCompany(e.Admin(), archived, nil); err != nil {
		t.Fatal(err)
	}
	if reason := refusedFor(t, undoEntry(t, e, "company", archived.UUID, receipt)); reason != ReasonRecordArchived {
		t.Fatalf("an archived company's lookup answered %s, want %s", reason, ReasonRecordArchived)
	}

	merged := technicalUndoCompany(t, e, "Merged Lookup GmbH")
	survivor := technicalUndoCompany(t, e, "Survivor GmbH")
	mergedReceipt := applyUndoLookup(t, e, merged, undoObservation("mail_provider", "mail", "Mail"))
	survivorReceipt := applyUndoLookup(t, e, survivor, undoObservation("hosting_provider", "host", "Host"))
	if _, err := e.Contacts.MergeCompany(e.Admin(), merged, survivor, nil); err != nil {
		t.Fatal(err)
	}
	survivorFacts := technicalUndoFacts(t, e, survivor)
	if err := undoEntry(t, e, "company", merged.UUID, mergedReceipt); err == nil {
		t.Fatal("the merged-away company's lookup was undone, against rows that now belong to the survivor")
	}
	if reason := refusedFor(t, undoEntry(t, e, "company", survivor.UUID, survivorReceipt)); reason != ReasonLaterCompanyChange {
		t.Fatalf("the survivor's lookup answered %s after the merge moved facts onto it, want %s", reason, ReasonLaterCompanyChange)
	}
	if !sameFacts(t, survivorFacts, technicalUndoFacts(t, e, survivor)) {
		t.Fatal("a refused Undo after a merge changed the survivor's facts")
	}
}

// A reader who may not see the company learns nothing from the Undo route.
// The answer is the refusal every other hidden record gets, and nothing is
// written.
func TestTechnicalUndoOfAHiddenCompanyIsNotFound(t *testing.T) {
	e := integration.Setup(t)
	company := technicalUndoCompany(t, e, "Hidden Lookup GmbH")
	applyUndoLookup(t, e, company, undoObservation("mail_provider", "old", "Old mail"))
	receipt := applyUndoLookup(t, e, company, undoObservation("mail_provider", "new", "New mail"))
	held := technicalUndoFacts(t, e, company)
	blind := e.As(e.Rep3, []ids.UUID{e.Team2}, principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects:  map[string]principal.ObjectGrant{"contact": {Read: true, Update: true}},
		RowScope: principal.RowScopeTeam,
	})
	_, err := restoreSeamFor(e).Restore(blind, "company", company.UUID, receipt, currentVersion(t, e, "company", company.UUID))
	if !errors.Is(err, apperrors.ErrNotFound) && !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a reader without the company answered %v, want not-found or permission denied", err)
	}
	if refusal, refused := errors.AsType[RefusedRestore](err); refused {
		t.Fatalf("the hidden company's Undo gave a reason (%s), which tells the reader the change exists", refusal.Reason)
	}
	if !sameFacts(t, held, technicalUndoFacts(t, e, company)) {
		t.Fatal("a hidden reader's Undo changed the facts")
	}
}

// A lookup recorded before receipts kept the replaced rows says so. Nothing
// is invented for it.
func TestALegacyLookupReceiptSaysItHasNoSnapshot(t *testing.T) {
	e := integration.Setup(t)
	company := technicalUndoCompany(t, e, "Legacy Lookup GmbH")
	var answer Undoability
	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var err error
		answer, err = restoreSeamFor(e).evaluator.Evaluate(e.Admin(), tx, AuditRow{
			ID: ids.NewV7(), EntityType: "company", EntityID: company.UUID, Action: string(auditverb.Update),
			Evidence:   json.RawMessage(`{"source":"technical_lookup","technical":{"changes":1,"field_changes":[{"field":"mail_provider","before":"Old","after":"New"}]}}`),
			OccurredAt: lookupObservedAt,
		}, Advisory)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if answer.Undoable || answer.Reason != ReasonNoRestoreSnapshot {
		t.Fatalf("a legacy lookup answered %+v, want %s", answer, ReasonNoRestoreSnapshot)
	}
}

// A company write whose transaction began before the lookup and landed after
// it is a later change. Its audit row carries the older transaction start, so
// the guard cannot lean on that time. Facts are held by their own counter, so
// the added-and-removed case above needs no clock either.
func TestTechnicalUndoRefusesALaterWriteThatBeganBeforeTheLookup(t *testing.T) {
	cases := map[string]func(ctx context.Context, e *integration.Env, company ids.CompanyID) error{
		"a company edit": func(ctx context.Context, e *integration.Env, company ids.CompanyID) error {
			_, err := e.Contacts.UpdateCompany(ctx, company, contacts.UpdateCompanyInput{Description: new("Began early")})
			return err
		},
	}
	for name, write := range cases {
		t.Run(name, func(t *testing.T) {
			e := integration.Setup(t)
			company := technicalUndoCompany(t, e, "Early Writer GmbH")
			applyUndoLookup(t, e, company, undoObservation("mail_provider", "old", "Old mail"))
			began, release, done := make(chan struct{}), make(chan struct{}), make(chan error, 1)
			go func() {
				done <- database.WithWorkspaceSnapshot(e.Admin(), e.Pool, func(ctx context.Context) error {
					// The first statement fixes the transaction's start time.
					if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
						_, err := tx.Exec(ctx, `SELECT 1`)
						return err
					}); err != nil {
						return err
					}
					close(began)
					<-release
					return write(ctx, e, company)
				})
			}()
			<-began
			receipt := applyUndoLookup(t, e, company, undoObservation("mail_provider", "new", "New mail"))
			close(release)
			if err := <-done; err != nil {
				t.Fatalf("the early write failed: %v", err)
			}
			held := technicalUndoFacts(t, e, company)
			if answer := advisoryAnswer(e.Admin(), t, e, "company", company.UUID, receipt); answer.Undoable ||
				answer.Reason != string(ReasonLaterCompanyChange) {
				t.Fatalf("the preview answered %+v, want a refusal naming the later change", answer)
			}
			if reason := refusedFor(t, undoEntry(t, e, "company", company.UUID, receipt)); reason != ReasonLaterCompanyChange {
				t.Fatalf("the Undo answered %s, want %s", reason, ReasonLaterCompanyChange)
			}
			if !sameFacts(t, held, technicalUndoFacts(t, e, company)) {
				t.Fatal("a refused Undo changed the facts")
			}
			if reversals, events := reversalWriteShape(t, e, receipt); reversals != 0 || events != 0 {
				t.Fatalf("a refused Undo committed %d reversal rows and %d events", reversals, events)
			}
		})
	}
}

// A merge changes the survivor's facts, so a lookup taken on either company
// before it can no longer be put back. The survivor's counter moved. The
// retired company is archived with no counter left.
func TestAMergeRefusesTheUndoOfALookupOnEitherCompany(t *testing.T) {
	e := integration.Setup(t)
	survivor := technicalUndoCompany(t, e, "Merge Survivor GmbH")
	retired := technicalUndoCompany(t, e, "Merge Retired GmbH")
	survivorLookup := applyUndoLookup(t, e, survivor, undoObservation("hosting_provider", "host", "Host"))
	retiredLookup := applyUndoLookup(t, e, retired, undoObservation("hosting_provider", "host", "Host"))
	if _, err := e.Contacts.MergeCompany(e.Admin(), retired, survivor, nil); err != nil {
		t.Fatal(err)
	}
	held := technicalUndoFacts(t, e, survivor)
	if answer := advisoryAnswer(e.Admin(), t, e, "company", survivor.UUID, survivorLookup); answer.Undoable ||
		answer.Reason != string(ReasonLaterCompanyChange) {
		t.Fatalf("the survivor's lookup previewed %+v after the merge, want a refusal naming the later change", answer)
	}
	if reason := refusedFor(t, undoEntry(t, e, "company", survivor.UUID, survivorLookup)); reason != ReasonLaterCompanyChange {
		t.Fatalf("the survivor's lookup answered %s after the merge, want %s", reason, ReasonLaterCompanyChange)
	}
	if reason := refusedFor(t, undoEntry(t, e, "company", retired.UUID, retiredLookup)); reason != ReasonRecordArchived {
		t.Fatalf("the retired company's lookup answered %s after the merge, want %s", reason, ReasonRecordArchived)
	}
	if !sameFacts(t, held, technicalUndoFacts(t, e, survivor)) {
		t.Fatal("a refused Undo after a merge changed the survivor's facts")
	}
	if left := e.WsCount(t, "SELECT count(*) FROM company_fact_revision WHERE company_id = $1", retired.UUID); left != 0 {
		t.Fatalf("the retired company kept %d fact revision rows", left)
	}
	for _, receipt := range []ids.UUID{survivorLookup, retiredLookup} {
		if reversals, events := reversalWriteShape(t, e, receipt); reversals != 0 || events != 0 {
			t.Fatalf("a refused Undo committed %d reversal rows and %d events", reversals, events)
		}
	}
}

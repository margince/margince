// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// errMoveRefused stands for any database error during a legacy evidence move.
var errMoveRefused = errors.New("the database refused the statement")

// refusingMoveTx answers its failAt-th Exec with errMoveRefused, and every
// Exec before it with success.
type refusingMoveTx struct {
	pgx.Tx
	failAt int
	calls  int
}

func (tx *refusingMoveTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	tx.calls++
	if tx.calls == tx.failAt {
		return pgconn.CommandTag{}, errMoveRefused
	}
	return pgconn.NewCommandTag("UPDATE 0"), nil
}

// A move stops at the first statement the database refuses, whichever of the
// flag, a table's move or the flag's reset it is.
func TestALegacyMoveReturnsTheStatementTheDatabaseRefused(t *testing.T) {
	ctx := context.Background()
	subject := ids.NewV7()
	statements, _ := legacyEvidenceMoves(entityCompany, "$1")
	for failAt := 1; failAt <= len(statements)+2; failAt++ {
		tx := &refusingMoveTx{failAt: failAt}
		if err := moveLegacyEvidence(ctx, tx, entityCompany, subject); !errors.Is(err, errMoveRefused) {
			t.Errorf("refusing statement %d: err = %v, want the refusal", failAt, err)
		}
		if tx.calls != failAt {
			t.Errorf("refusing statement %d: %d statements ran, want none after the refusal", failAt, tx.calls)
		}
	}
}

func TestALegacyMoveOfNoSubjectsRunsNothing(t *testing.T) {
	tx := &refusingMoveTx{failAt: 1}
	if err := moveLegacyEvidence(context.Background(), tx, entityContact); err != nil || tx.calls != 0 {
		t.Errorf("a move of no subjects = %v after %d statements, want nil after none", err, tx.calls)
	}
}

func TestAKindWithNoLegacyEvidenceIsRefused(t *testing.T) {
	if err := moveLegacyEvidence(context.Background(), &refusingMoveTx{}, "deal", ids.NewV7()); err == nil {
		t.Error("a move of a deal's evidence = nil, want a refusal: only companies and contacts have legacy evidence")
	}
}

// Every writer moves its subject's legacy rows before it writes, and a move
// the database refuses stops the writer there.
func TestEveryEvidenceWriterStopsWhenItsLegacyMoveIsRefused(t *testing.T) {
	ctx := context.Background()
	system := principal.SystemActing(ctx, "system:test")
	company := ids.New[ids.CompanyKind]()
	contact := ids.New[ids.ContactKind]()
	writers := map[string]func(pgx.Tx) error{
		"revokeSignatureEvidence": func(tx pgx.Tx) error { return revokeSignatureEvidence(ctx, tx, contact, fieldTitle) },
		"addSignatureSupportTx": func(tx pgx.Tx) error {
			_, err := addSignatureSupportTx(ctx, tx, contact, fieldPhone, "+4930", "+4930", "activity:x", "sig")
			return err
		},
		"markStreetSupportTx": func(tx pgx.Tx) error { return markStreetSupportTx(ctx, tx, contact, "activity:x") },
		"dropProfileFieldSupportTx": func(tx pgx.Tx) error {
			_, err := dropProfileFieldSupportTx(ctx, tx, "activity:x")
			return err
		},
		"readUndo": func(tx pgx.Tx) error {
			_, err := readUndo(ctx, tx, contact, fieldTitle, "")
			return err
		},
		"retractField": func(tx pgx.Tx) error { return retractField(ctx, tx, contact, FillRetraction{}, fieldTitle) },
		"carryCompanyEnrichment": func(tx pgx.Tx) error {
			return carryCompanyEnrichment(ctx, tx, company, ids.New[ids.CompanyKind]())
		},
		"writeCompanyFields": func(tx pgx.Tx) error {
			_, err := writeCompanyFields(ctx, tx, company, "human:x", nil)
			return err
		},
		"writeMeetingFieldsTx": func(tx pgx.Tx) error {
			return writeMeetingFieldsTx(ctx, tx, company, ids.New[ids.ActivityKind](), "agent:x", nil, &meetingFieldImages{})
		},
		"retractUnstatedMeetingFields": func(tx pgx.Tx) error {
			return retractUnstatedMeetingFields(ctx, tx, ids.New[ids.ActivityKind](), nil, []ids.UUID{company.UUID}, nil)
		},
		"RetractStaleMeetingProfilesTx": func(tx pgx.Tx) error {
			_, err := RetractStaleMeetingProfilesTx(system, tx, 10)
			return err
		},
		"applyEvidenceFieldsWithOverwrite": func(tx pgx.Tx) error {
			_, err := applyEvidenceFieldsWithOverwrite(ctx, tx, ids.WorkspaceID{}, company, companySourceSiteRead, "agent:x", nil, nil)
			return err
		},
	}
	for name, write := range writers {
		t.Run(name, func(t *testing.T) {
			tx := &refusingMoveTx{failAt: 1}
			if err := write(tx); !errors.Is(err, errMoveRefused) {
				t.Errorf("err = %v, want the refused move", err)
			}
			if tx.calls != 1 {
				t.Errorf("%d statements ran, want the writer to stop at the refused move", tx.calls)
			}
		})
	}
}

// The data task refuses a caller that is not the system and a cursor it
// cannot read, before it reads a row.
func TestTheLegacyEvidenceTaskRefusesBeforeItReads(t *testing.T) {
	legacy := legacyEvidenceTables[0]
	if _, err := moveLegacyEvidenceBatch(context.Background(), nil, legacy, datatask.BatchIn{Limit: 1}); err == nil {
		t.Error("a batch outside the system = nil error, want a refusal")
	}
	system := principal.SystemActing(context.Background(), "system:test")
	bad := datatask.BatchIn{Limit: 1, Cursor: []byte("not a cursor")}
	if _, err := moveLegacyEvidenceBatch(system, nil, legacy, bad); err == nil {
		t.Error("a batch with an unreadable cursor = nil error, want a refusal")
	}
	if _, _, err := countLegacyEvidence(context.Background(), nil, legacy, 10); err == nil {
		t.Error("an estimate by a caller with no data task grant = nil error, want a refusal")
	}
	task := LegacyEvidenceDataTask()
	if len(task.Steps) != len(legacyEvidenceTables) {
		t.Errorf("the task has %d steps, want one per legacy table (%d)", len(task.Steps), len(legacyEvidenceTables))
	}
}

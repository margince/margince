// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A module that refuses at the write answers with the reason the button would
// have shown up front, so a refusal found late reads the same as one found
// early. Anything that is not a refusal travels on unchanged.
func TestAModuleRefusalReadsAsTheReasonTheButtonShows(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want Reason
	}{
		{"erased since", &storekit.RestoreRefusal{Reason: storekit.RestoreErased}, ReasonBehindErasureBoundary},
		{"brought back since", &storekit.RestoreRefusal{Reason: storekit.RestoreNotArchived}, ReasonSuperseded},
		{"merged away", &storekit.RestoreRefusal{Reason: storekit.RestoreMerged}, ReasonNotRestorableByThisPath},
		{"address taken", &storekit.RestoreRefusal{Reason: storekit.RestoreValueTaken, Detail: "email"}, ReasonNotRestorableByThisPath},
		{"field changed since", &contacts.FillRetractionRefusal{Moved: []string{"title"}}, ReasonSuperseded},
		{"value replaced", &contacts.FillRetractionRefusal{Replaced: []string{"phone"}}, ReasonNotRestorableByThisPath},
		{"no longer promoted", &contacts.NotPromotedError{}, ReasonSuperseded},
		{"contact on a live deal", &contacts.ContactHasDealError{}, ReasonNotRestorableByThisPath},
		{"colleague worked on it", &contacts.HumanTouchedError{EntityType: "contact"}, ReasonSuperseded},
	}
	for _, c := range cases {
		var refusal RefusedRestore
		if !errors.As(inverseWriteRefusal(fmt.Errorf("wrapped: %w", c.err)), &refusal) {
			t.Errorf("%s: not a refusal", c.name)
			continue
		}
		if refusal.Reason != c.want {
			t.Errorf("%s: reason = %q, want %q", c.name, refusal.Reason, c.want)
		}
	}
	boom := errors.New("the database went away")
	if got := inverseWriteRefusal(boom); !errors.Is(got, boom) {
		t.Errorf("a fault came back as %v, want it unchanged", got)
	}
}

// An un-archive is recognised by the archive it names, so a field restore is
// never mistaken for one and archived again.
func TestOnlyAnUnarchiveIsRedoneByArchiving(t *testing.T) {
	unarchive := AuditRow{EntityType: "company", Action: actionRestore, Evidence: []byte(`{"restores_archive":"x"}`)}
	field := AuditRow{EntityType: "company", Action: actionRestore, Evidence: []byte(`{"undid_audit_log_id":"x"}`)}
	if inverseOf(unarchive) != inverseRearchive {
		t.Error("an un-archive is not redone by archiving")
	}
	if inverseOf(field) != inverseNone {
		t.Error("a field restore is read as an un-archive")
	}
	if undoGrantFor(unarchive) != "delete" || undoGrantFor(field) != "update" {
		t.Errorf("grants = %q / %q, want delete for the archive and update for the field", undoGrantFor(unarchive), undoGrantFor(field))
	}
}

// systemSeatCtx is a seat holding every grant, so the object gate passes and the
// branches after it are what a case is about.
func systemSeatCtx() context.Context {
	return principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalSystem, ID: "system:test"})
}

// The refusals an inverse decides from the ports alone, before it reads the
// database: who may act, whether it is undone, and the record's archived state.
func TestAnInverseIsRefusedFromItsPortsBeforeItReadsTheTrail(t *testing.T) {
	created := AuditRow{ID: ids.NewV7(), EntityType: "contact", EntityID: ids.NewV7(), Action: actionCreate}
	archive := AuditRow{ID: ids.NewV7(), EntityType: "company", EntityID: ids.NewV7(), Action: actionArchive}
	fill := AuditRow{
		ID: ids.NewV7(), EntityType: "contact", EntityID: ids.NewV7(), Action: auditActionUpdate,
		Before:   json.RawMessage(`{"title":null}`),
		After:    json.RawMessage(`{"title":"filled"}`),
		Evidence: json.RawMessage(`{"source":"capture_enrich","source_ref":"activity:x","confirmed":["title"]}`),
	}
	archived := func(v bool) func(context.Context, pgx.Tx, string, ids.UUID) (bool, error) {
		return func(context.Context, pgx.Tx, string, ids.UUID) (bool, error) { return v, nil }
	}
	notMine := func(context.Context, pgx.Tx, string, ids.UUID) error { return errRecordNotWritable }
	cases := []struct {
		name string
		e    Evaluator
		row  AuditRow
		want Reason
	}{
		{"not the caller's", Evaluator{Writable: notMine}, created, ReasonNotWritableByCaller},
		{"create of an archived record", Evaluator{Archived: archived(true)}, created, ReasonRecordArchived},
		{"archive of a live record", Evaluator{Archived: archived(false)}, archive, ReasonSuperseded},
		{"a fill that only confirmed", Evaluator{}, fill, ReasonNotRestorableByThisPath},
	}
	for _, c := range cases {
		answer, err := c.e.Evaluate(systemSeatCtx(), nil, c.row, Advisory)
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if answer.Reason != c.want {
			t.Errorf("%s: reason = %q, want %q", c.name, answer.Reason, c.want)
		}
	}
	boom := errors.New("the scope read failed")
	failing := Evaluator{Writable: func(context.Context, pgx.Tx, string, ids.UUID) error { return boom }}
	if _, err := failing.Evaluate(systemSeatCtx(), nil, created, Advisory); !errors.Is(err, boom) {
		t.Errorf("a failed scope read answered %v, want the fault", err)
	}
	if err := (recordInverses{}).perform(systemSeatCtx(), nil, created, inverseNone, 1); err == nil {
		t.Error("an entry with no module verb was performed")
	}
	if err := (recordInverses{}).unarchive(systemSeatCtx(), nil, AuditRow{EntityType: "project"}, 1); err == nil {
		t.Error("a project was un-archived")
	}
}

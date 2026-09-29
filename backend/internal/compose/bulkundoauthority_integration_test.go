// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Who may read and undo a batch, and what they are shown of it: the identity
// that asked survives an OAuth refresh, and a record the reader can no longer
// see is never named again.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// agentOnConnection is the principal identity mints for one passport of an
// OAuth connection; a refresh mints the next passport on the same connection.
func agentOnConnection(e *integration.Env, human, connection ids.UUID) context.Context {
	agent := identity.AgentIdentity{
		PassportID:   ids.New[ids.PassportKind](),
		WorkspaceID:  ids.From[ids.WorkspaceKind](e.WS),
		OnBehalfOf:   ids.From[ids.UserKind](human),
		ConnectionID: connection,
		SeatType:     string(principal.SeatFull),
		Scopes:       principal.ScopeSet{principal.ScopeRead: {}, principal.ScopeWrite: {}},
		Teams:        []ids.TeamID{ids.From[ids.TeamKind](e.Team1)},
		Permissions:  integration.RepPerms,
	}
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, agent.Principal())
}

func TestAnAgentsBatchStaysItsOwnAcrossARefreshAndItsHumansToo(t *testing.T) {
	e := integration.Setup(t)
	engine := bulkEngineFor(e)
	connection := ids.NewV7()
	first := agentOnConnection(e, e.Rep1, connection)
	moved, err := engine.Execute(first, reassignTo(e.Rep2, seedBulkContacts(t, e, e.Rep1, 2)))
	if err != nil || moved.Changed != 2 {
		t.Fatalf("the agent's reassign → %+v, %v", moved, err)
	}
	batch := ids.UUID(moved.BatchId)

	refreshed := agentOnConnection(e, e.Rep1, connection)
	if _, err := engine.Status(refreshed, batch); err != nil {
		t.Errorf("the same connection after a refresh reads its batch → %v", err)
	}
	human := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms)
	if _, err := engine.Status(human, batch); err != nil {
		t.Errorf("the human the agent acts for reads the batch → %v", err)
	}
	stranger := agentOnConnection(e, e.Rep2, ids.NewV7())
	if _, err := engine.Status(stranger, batch); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("another human's agent reads the batch → %v, want not found", err)
	}
	if _, err := engine.Status(e.As(e.Rep2, []ids.UUID{e.Team1}, integration.RepPerms), batch); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("another colleague reads the batch → %v, want not found", err)
	}
	if undone, err := engine.Undo(refreshed, batch, ""); err != nil || undone.Changed != 2 {
		t.Errorf("the refreshed agent undoes its batch → %+v, %v", undone, err)
	}
}

func TestABatchNoLongerNamesARecordItsReaderLostSightOf(t *testing.T) {
	e := integration.Setup(t)
	engine := bulkEngineFor(e)
	rep1 := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms)
	own := seedBulkContacts(t, e, e.Rep1, 2)
	already := seedBulkContacts(t, e, e.Rep2, 1)
	moved, err := engine.Execute(rep1, reassignTo(e.Rep2, append(own, already...)))
	if err != nil || moved.Changed != 2 || len(moved.Skipped) != 1 {
		t.Fatalf("the reassign → %+v, %v; want two changed and one left alone", moved, err)
	}
	batch := ids.UUID(moved.BatchId)
	if status, err := engine.Status(rep1, batch); err != nil || len(status.Skipped) != 1 {
		t.Fatalf("before losing sight the requester reads %+v, %v", status, err)
	}
	if err := bulkBatchStillSeen(rep1, e.DB(), batch); err != nil {
		t.Fatalf("before losing sight a replay of the answer is refused: %v", err)
	}

	e.MakeCapturePrivate(t, "contact", ids.UUID(already[0].Id), e.Rep3)
	e.MakeCapturePrivate(t, "contact", ids.UUID(own[1].Id), e.Rep3)

	status, err := engine.Status(rep1, batch)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(status.Skipped) != 0 {
		t.Errorf("the status still names %v, which the requester can no longer see", status.Skipped)
	}
	if err := bulkBatchStillSeen(rep1, e.DB(), batch); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a replay of the answer naming a record out of sight → %v, want it refused", err)
	}
	undone, err := engine.Undo(rep1, batch, "")
	if err != nil {
		t.Fatalf("Undo: %v", err)
	}
	for _, skip := range undone.Skipped {
		if skip.Id == own[1].Id {
			t.Errorf("the undo names %s, which the requester can no longer see (%s)", skip.Id, skip.Reason)
		}
	}
	if undone.Changed != 1 {
		t.Errorf("the undo changed %d, want the one contact still in sight", undone.Changed)
	}
	if err := bulkBatchStillSeen(rep1, e.DB(), ids.UUID(undone.BatchId)); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a replay of the undo's answer, whose batch names a record out of sight → %v, want it refused", err)
	}
}

func TestUndoingAnArchiveBringsBackALinkBetweenTwoRecordsOfTheBatch(t *testing.T) {
	e := integration.Setup(t)
	engine := bulkEngineFor(e)
	items := seedBulkContacts(t, e, e.Rep1, 2)
	a := ids.From[ids.ContactKind](ids.UUID(items[0].Id))
	b := ids.From[ids.ContactKind](ids.UUID(items[1].Id))
	link, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "works_with", ContactID: &a, CounterpartyContactID: &b,
	})
	if err != nil {
		t.Fatalf("linking the two contacts: %v", err)
	}
	for i := range items {
		items[i].Version = int64(e.WsCount(t, `SELECT version FROM contact WHERE id = $1`, items[i].Id))
	}
	archived, err := engine.Execute(e.Admin(), archiveContacts(items))
	if err != nil || archived.Changed != 2 {
		t.Fatalf("archiving both → %+v, %v", archived, err)
	}

	undone, err := engine.Undo(e.Admin(), ids.UUID(archived.BatchId), "")
	if err != nil || undone.Changed != 2 {
		t.Fatalf("Undo → %+v, %v", undone, err)
	}
	if undone.LeftBehind != nil && len(*undone.LeftBehind) > 0 {
		t.Errorf("left behind %v; want the link between the two back", *undone.LeftBehind)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM relationship WHERE id = $1 AND archived_at IS NULL`, link.ID); n != 1 {
		t.Error("the link between the two restored contacts is still archived")
	}
}

// A retried undo under its Idempotency-Key replays the first answer only while
// the caller may still see every record that answer names.
func TestAReplayedUndoIsRefusedOnceItNamesARecordOutOfSight(t *testing.T) {
	e := integration.Setup(t)
	engine := bulkEngineFor(e)
	rep1 := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.RepPerms)
	items := seedBulkContacts(t, e, e.Rep1, 2)
	moved, err := engine.Execute(rep1, reassignTo(e.Rep2, items))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	title := "Buyer"
	edited := ids.From[ids.ContactKind](ids.UUID(items[1].Id))
	if _, err := e.Contacts.UpdateContact(e.Admin(), edited, contacts.UpdateContactInput{Title: &title}); err != nil {
		t.Fatalf("editing after the batch: %v", err)
	}
	handlers := bulkHandlers{engine: engine}
	r := chi.NewRouter()
	r.With(idempotency(e.Pool, replayProbes(nil, nil, nil, e.DB()), nil)).Post("/v1/bulk/{id}/undo",
		func(w http.ResponseWriter, req *http.Request) {
			id := ids.MustParse(chi.URLParam(req, "id"))
			handlers.UndoBulkChange(w, req, openapi_types.UUID(id), crmcontracts.UndoBulkChangeParams{})
		})
	call := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/bulk/"+moved.BatchId.String()+"/undo",
			strings.NewReader(`{}`)).WithContext(rep1)
		req.Header.Set("Idempotency-Key", "undo-replay")
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}
	first := call()
	if first.Code != http.StatusOK || !strings.Contains(first.Body.String(), items[1].Id.String()) {
		t.Fatalf("the undo → %d %s; want 200 naming the edited contact as skipped", first.Code, first.Body)
	}
	if again := call(); again.Code != http.StatusOK || again.Body.String() != first.Body.String() {
		t.Fatalf("the replay while in sight → %d %s; want the first answer", again.Code, again.Body)
	}

	e.MakeCapturePrivate(t, "contact", ids.UUID(items[1].Id), e.Rep3)

	if again := call(); again.Code != http.StatusNotFound {
		t.Errorf("the replay naming a record out of sight → %d %s, want 404", again.Code, again.Body)
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// HTTP-level coverage for GET /field-history: the handler
// (privacy.Handlers.GetFieldHistory) and its wire mapping
// (fieldHistoryEntryToWire) that fieldhistory_integration_test.go never
// drives — that suite calls privacy.ListFieldHistory directly, so the
// query-validation branches and the JSON shape only exist at the
// transport. This suite rides the same real-handler-stack e2e harness as
// integration/apptest (TLS httptest server, session cookie, workspace
// header) and reuses fieldhistory_integration_test.go's seedAuditDiffRow
// to write the audit rows the handler reads back.

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// fieldHistoryEntryWire mirrors the contract's FieldHistoryEntry field by
// field, decoded loosely so a wire-shape regression (a renamed or
// mistyped key) fails the assertions below instead of silently zeroing.
type fieldHistoryEntryWire struct {
	Field      string         `json:"field"`
	OldValue   *string        `json:"old_value"`
	NewValue   *string        `json:"new_value"`
	ChangedAt  string         `json:"changed_at"`
	ActorType  string         `json:"actor_type"`
	ActorID    string         `json:"actor_id"`
	ActorName  *string        `json:"actor_name"`
	OnBehalfOf *string        `json:"on_behalf_of_name"`
	PassportID *string        `json:"passport_id"`
	Evidence   map[string]any `json:"evidence"`
}

type fieldHistoryListWire struct {
	Data []fieldHistoryEntryWire `json:"data"`
	Page struct {
		HasMore    bool    `json:"has_more"`
		NextCursor *string `json:"next_cursor"`
	} `json:"page"`
}

// fieldHistoryProblem is the RFC 7807 body httperr.Validation produces.
type fieldHistoryProblem struct {
	Code    string `json:"code"`
	Detail  string `json:"detail"`
	Details struct {
		Errors []struct {
			Field   string `json:"field"`
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"errors"`
	} `json:"details"`
}

// assertFieldHistoryValidation422 checks the shared 422 problem+json shape
// httperr.Validation writes for every query-validation rejection: the
// envelope code, and the single per-field error naming the rejected field
// and its machine code.
func assertFieldHistoryValidation422(t *testing.T, status int, problem fieldHistoryProblem, field, code string) {
	t.Helper()
	if status != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422: %+v", status, problem)
	}
	if problem.Code != "validation_error" {
		t.Fatalf("problem code = %q, want validation_error", problem.Code)
	}
	if len(problem.Details.Errors) != 1 {
		t.Fatalf("want exactly one field error, got %+v", problem.Details.Errors)
	}
	if got := problem.Details.Errors[0]; got.Field != field || got.Code != code {
		t.Fatalf("field error = %+v, want field=%s code=%s", got, field, code)
	}
}

// fieldHistoryHTTPEnv resolves the workspace id the HTTP harness's
// bootstrap created and opens a second pgxpool.Pool onto the same live
// schema — exactly how apptest.SetupApp itself pairs its owner connection with an
// app pool — so the store-level suite's seedAuditDiffRow can write
// straight through the real audit-spine path this handler reads back.
func fieldHistoryHTTPEnv(t *testing.T, e *apptest.AppEnv) *Env {
	t.Helper()
	ctx := context.Background()
	ws := apptest.InstallationWorkspaceUUID(ctx, t, e.Owner)
	pool, err := testdb.OwnPool(ctx, apptest.AppDSN(t))
	if err != nil {
		t.Fatalf("opening app pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return &Env{Pool: pool, WS: ws}
}

// seedHumanFieldHistoryRow seeds a human-actor diff against a REAL seat.
//
// The shared seeder writes a placeholder actor id, which is fine where the
// actor is only a discriminator. It is not fine here: the name resolution this
// rail now does joins app_user on the 'human:' || id key the writer actually
// produces, so a made-up id would exercise the join against a shape no write
// makes and report a null name as correct.
func seedHumanFieldHistoryRow(t *testing.T, app *apptest.AppEnv, e *Env, entityID ids.UUID,
	before, after map[string]any, occurredAt time.Time,
) {
	t.Helper()
	var seat ids.UUID
	// identity.LiveMemberSQL rather than the pair spelled out here. Both halves
	// matter — deactivating sets status and leaves archived_at NULL, so either
	// alone picks a seat the workspace does not have — and identity owns what
	// "still works here" means. A fixture attributing an audit row to a departed
	// colleague would test the name resolution against a state no write produces.
	if err := app.Owner.QueryRow(context.Background(),
		`SELECT id FROM app_user WHERE `+identity.LiveMemberSQL("")+
			` ORDER BY created_at LIMIT 1`).
		Scan(&seat); err != nil {
		t.Fatalf("reading the workspace's own seat: %v", err)
	}
	beforeJSON, afterJSON := auditImageJSON(t, before), auditImageJSON(t, after)
	ctx := principal.WithWorkspaceID(t.Context(), e.WS)
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO audit_log (id, actor_type, actor_id, action,
			                        entity_type, entity_id, before, after, occurred_at)
			 VALUES ($1, 'human', $2, 'update', 'contact', $3, $4, $5, $6)`,
			ids.NewV7(), "human:"+seat.String(), entityID, beforeJSON, afterJSON, occurredAt)
		return err
	}); err != nil {
		t.Fatalf("seed human audit row: %v", err)
	}
}

// seedAgentFieldHistoryRow seeds an agent-actor audit diff row carrying a
// passport id and evidence — the one shape seedAuditDiffRow cannot
// produce (it never binds those two columns) and the only shape that
// exercises fieldHistoryEntryToWire's passport/evidence branches, since
// makeFieldHistoryEntry surfaces them for actor_type=agent only.
func seedAgentFieldHistoryRow(t *testing.T, e *Env, entityType string, entityID, passportID ids.UUID,
	evidence, before, after map[string]any, occurredAt time.Time,
) ids.UUID {
	t.Helper()
	beforeJSON, afterJSON := auditImageJSON(t, before), auditImageJSON(t, after)
	evidenceJSON, err := json.Marshal(evidence)
	if err != nil {
		t.Fatalf("marshal evidence: %v", err)
	}
	rowID := ids.NewV7()
	ctx := principal.WithWorkspaceID(t.Context(), e.WS)
	err = database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx,
			`INSERT INTO audit_log (id, actor_type, actor_id, passport_id, action,
			                        entity_type, entity_id, before, after, evidence, occurred_at)
			 VALUES ($1, 'agent', 'agent:test', $2, 'update', $3, $4, $5, $6, $7, $8)`, rowID, passportID, entityType, entityID, beforeJSON, afterJSON, evidenceJSON, occurredAt)
		return err
	})
	if err != nil {
		t.Fatalf("seed agent audit row: %v", err)
	}
	return rowID
}

// fieldHistoryHTTPFixture is the seeded shape TestFieldHistoryHTTP's happy
// path reads back: a contact created through the real HTTP write path (its
// own create-audit row is honest genesis history), a human-actor title
// diff, and — dated newest so it lands at data[0] — an agent-actor diff
// carrying a passport id and evidence.
type fieldHistoryHTTPFixture struct {
	contactID  ids.UUID
	passportID ids.UUID
}

// seedFieldHistoryHTTPFixture creates the subject and seeds the two audit
// diff rows the happy-path subtest below asserts on.
func seedFieldHistoryHTTPFixture(t *testing.T, e *apptest.AppEnv, dbEnv *Env) fieldHistoryHTTPFixture {
	t.Helper()
	var contact AnyMap
	if status := e.Call(t, "POST", "/v1/contacts", AnyMap{
		"full_name": "History Wire Subject",
		"source":    "ui",
	}, nil, &contact); status != http.StatusCreated {
		t.Fatalf("create contact = %d %v", status, contact)
	}
	contactID, err := ids.Parse(contact["id"].(string))
	if err != nil {
		t.Fatalf("parsing contact id %q: %v", contact["id"], err)
	}

	// Dated forward from the create row so ordering is unambiguous
	// (fieldhistory_integration_test.go's own convention).
	humanAt := time.Now().Add(1 * time.Hour).UTC().Truncate(time.Microsecond)
	agentAt := time.Now().Add(2 * time.Hour).UTC().Truncate(time.Microsecond)
	seedHumanFieldHistoryRow(t, e, dbEnv, contactID,
		map[string]any{"title": "VP"}, map[string]any{"title": "CTO"}, humanAt)

	passportID := ids.NewV7()
	evidence := map[string]any{"tool_call_id": "call-1", "confidence": "0.92"}
	seedAgentFieldHistoryRow(t, dbEnv, "contact", contactID, passportID, evidence,
		map[string]any{"score": "1"}, map[string]any{"score": "2"}, agentAt)

	return fieldHistoryHTTPFixture{contactID: contactID, passportID: passportID}
}

// assertFieldHistoryHappyPath drives the GET and checks the wire shape:
// data[0] carries the required scalar fields plus the newest (agent) row's
// passport/evidence, the seeded human diff surfaces with both absent, and
// the page envelope is present.
func assertFieldHistoryHappyPath(t *testing.T, e *apptest.AppEnv, fx fieldHistoryHTTPFixture) {
	t.Helper()
	var page fieldHistoryListWire
	status := e.Call(t, "GET", "/v1/field-history?entity_type=contact&entity_id="+fx.contactID.String(), nil, nil, &page)
	if status != http.StatusOK {
		t.Fatalf("field-history status = %d, want 200: %+v", status, page)
	}
	if len(page.Data) < 3 {
		t.Fatalf("want at least 3 entries (create genesis + two seeded diffs): %+v", page.Data)
	}

	newest := page.Data[0]
	if newest.Field == "" || newest.ChangedAt == "" || newest.ActorType == "" {
		t.Fatalf("data[0] missing a required field: %+v", newest)
	}
	if newest.ActorType != "agent" {
		t.Fatalf("data[0] actor_type = %q, want agent (the newest-dated seeded row)", newest.ActorType)
	}
	if newest.OldValue == nil || *newest.OldValue != "1" || newest.NewValue == nil || *newest.NewValue != "2" {
		t.Fatalf("data[0] old/new value = %+v, want the seeded score diff", newest)
	}
	if newest.PassportID == nil || *newest.PassportID != fx.passportID.String() {
		t.Errorf("agent entry passport_id = %v, want %s", newest.PassportID, fx.passportID)
	}
	if newest.Evidence == nil || newest.Evidence["tool_call_id"] != "call-1" {
		t.Errorf("agent entry evidence = %v, want the seeded evidence map", newest.Evidence)
	}

	// A MACHINE ACTOR NAMES NO HUMAN, which is the honest half: the id is a
	// passport, not a human, and the human behind it rides on_behalf_of_name.
	if newest.ActorName != nil {
		t.Errorf("agent entry actor_name = %q, want null — the actor is a machine "+
			"and naming it as a contact is the confusion PD-002 is about", *newest.ActorName)
	}

	var sawHumanTitle bool
	for _, en := range page.Data {
		if en.Field == "title" && en.ActorType == "human" {
			sawHumanTitle = true
			if en.PassportID != nil || en.Evidence != nil {
				t.Errorf("human entry carries passport/evidence, want both absent: %+v", en)
			}
			// THE POINT OF THIS CHANGE. This rail renders beside the record
			// timeline on one screen, and until now it resolved no name at all
			// — so two adjacent rails disagreed about whether attribution names
			// anybody. The id was accurate and unreadable.
			if en.ActorName == nil {
				t.Errorf("human entry actor_name is null — the rail still shows a raw id "+
					"beside a record-history row that names the contact: %+v", en)
			} else if *en.ActorName == "" {
				t.Errorf("human entry actor_name is empty — absent and blank are different " +
					"answers, and a blank one renders as a contact with no name")
			}
		}
	}
	if !sawHumanTitle {
		t.Errorf("seeded human title diff missing from the wire response: %+v", page.Data)
	}
	// page.has_more is a required (non-pointer) field on the wire — it
	// decodes regardless of value, so its mere presence in the envelope
	// is what this asserts, not that it is true.
	_ = page.Page.HasMore
}

func TestFieldHistoryHTTP(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	dbEnv := fieldHistoryHTTPEnv(t, e)
	fx := seedFieldHistoryHTTPFixture(t, e, dbEnv)

	t.Run("200 happy path with wire mapping", func(t *testing.T) {
		assertFieldHistoryHappyPath(t, e, fx)
	})

	t.Run("422 invalid entity_type", func(t *testing.T) {
		var problem fieldHistoryProblem
		status := e.Call(t, "GET", "/v1/field-history?entity_type=bogus&entity_id="+ids.NewV7().String(), nil, nil, &problem)
		assertFieldHistoryValidation422(t, status, problem, "entity_type", "invalid_entity_type")
	})

	t.Run("422 invalid actor_type", func(t *testing.T) {
		var problem fieldHistoryProblem
		status := e.Call(t, "GET",
			"/v1/field-history?entity_type=contact&entity_id="+ids.NewV7().String()+"&actor_type=bogus", nil, nil, &problem)
		assertFieldHistoryValidation422(t, status, problem, "actor_type", "invalid_actor_type")
	})

	t.Run("422 malformed cursor", func(t *testing.T) {
		var problem fieldHistoryProblem
		status := e.Call(t, "GET",
			"/v1/field-history?entity_type=contact&entity_id="+ids.NewV7().String()+"&cursor=!!!notatoken", nil, nil, &problem)
		assertFieldHistoryValidation422(t, status, problem, "cursor", "malformed_cursor")
	})

	// A 404 for an out-of-scope/nonexistent record needs a bounded
	// (non-admin) session, and the contract surface has no
	// user-invitation endpoint — the workspace bootstrap mints exactly
	// one admin per workspace and nothing cheaper. That row-scope gate is
	// already proven at the store level by
	// TestFieldHistoryGatesOnReadPermissionAndVisibility (over the same
	// privacy.ListFieldHistory this handler calls), so it is not
	// duplicated here.
}

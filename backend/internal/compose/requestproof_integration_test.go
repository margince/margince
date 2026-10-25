// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// proofEnv is one installation with the consent handlers wired to the real
// erasure engine, as the server wires them.
type proofEnv struct {
	*integration.Env
	handlers consent.Handlers
	store    *consent.Store
}

func setupProofs(t *testing.T) proofEnv {
	t.Helper()
	e := integration.Setup(t)
	return proofEnv{
		Env:      e,
		handlers: consent.NewHandlers(e.DB()).WithEraser(privacy.NewEraser(e.DB())),
		store:    consent.NewStore(e.DB()),
	}
}

func (e proofEnv) openRequest(t *testing.T, kind, subjectRef string) ids.UUID {
	t.Helper()
	created, err := e.store.CreateDSR(e.Admin(), consent.CreateDSRInput{Kind: kind, SubjectRef: subjectRef})
	if err != nil {
		t.Fatalf("opening a %s request: %v", kind, err)
	}
	return created.ID
}

func (e proofEnv) patch(t *testing.T, id ids.UUID, body string) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPatch, "/v1/data-subject-requests/"+id.String(),
		strings.NewReader(body)).WithContext(e.Admin())
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	e.handlers.UpdateDataSubjectRequest(w, r, openapi_types.UUID(id))
	if w.Code != http.StatusOK {
		t.Fatalf("PATCH %s → %d: %s", body, w.Code, w.Body)
	}
}

func (e proofEnv) proofCount(t *testing.T, request ids.UUID) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM privacy_request_proof WHERE request_id = $1`, request)
}

func (e proofEnv) proofCounts(t *testing.T, request ids.UUID) map[string]int64 {
	t.Helper()
	var counts map[string]int64
	if err := json.Unmarshal([]byte(e.WsScalar(t,
		`SELECT erased_counts::text FROM privacy_request_proof WHERE request_id = $1`, request)), &counts); err != nil {
		t.Fatal(err)
	}
	return counts
}

// tombstoneCounts reads the counts the erasure engine certified for request on
// contact, so the proof can be held to the engine's own numbers.
func (e proofEnv) tombstoneCounts(t *testing.T, contact, request ids.UUID) map[string]int64 {
	t.Helper()
	var evidence map[string]any
	if err := json.Unmarshal([]byte(e.WsScalar(t, `
		SELECT evidence::text FROM audit_log
		 WHERE entity_type = 'contact' AND entity_id = $1 AND action = 'erase'
		   AND evidence->>'reason' = $2`, contact, "dsr:"+request.String())), &evidence); err != nil {
		t.Fatal(err)
	}
	out := map[string]int64{}
	for key, value := range evidence {
		if count, isCount := value.(float64); isCount {
			out[key] = int64(count)
		}
	}
	return out
}

func TestFulfillingAnErasureWritesOneProofWithTheErasureCountsAndTheOfficer(t *testing.T) {
	e := setupProofs(t)
	contact := e.SeedContact(t, "Henrike Proofsubject", nil)
	subject, body := "Loeschantrag Henrike", "Bitte alles loeschen, Ihre Henrike"
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "note", Subject: &subject, Body: &body, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	}); err != nil {
		t.Fatal(err)
	}
	request := e.openRequest(t, "erasure", contact.String())
	closing := `{"status":"fulfilled","resolution":"erased","identity_check":"id_document","requester_email":"Henrike@Proof.test"}`

	e.patch(t, request, closing)
	e.patch(t, request, closing)

	if n := e.proofCount(t, request); n != 1 {
		t.Fatalf("%d proofs after a fulfilment and its replay, want 1", n)
	}
	counts := e.proofCounts(t, request)
	if want := e.tombstoneCounts(t, contact, request); !maps.Equal(counts, want) {
		t.Fatalf("proof counts %v, want the erasure's own %v", counts, want)
	}
	if counts["activities_redacted"] != 1 || counts["contacts_anonymized"] != 1 {
		t.Fatalf("proof counts %v, want the one note and the one contact", counts)
	}
	got := e.WsScalar(t, `SELECT concat_ws('|', completed_by_type, completed_by_id, identity_check, result,
		request_kind, requester_email, erased_counts_known::text) FROM privacy_request_proof WHERE request_id = $1`, request)
	if want := "human|human:" + e.AdminUser.String() + "|id_document|fulfilled|erasure|henrike@proof.test|true"; got != want {
		t.Fatalf("proof = %s, want %s", got, want)
	}
	row := e.WsScalar(t, `SELECT row_to_json(p)::text FROM privacy_request_proof p WHERE request_id = $1`, request)
	for _, contactData := range []string{"Henrike Proofsubject", subject, body, contact.String()} {
		if strings.Contains(row, contactData) {
			t.Fatalf("the proof %s holds %q", row, contactData)
		}
	}
	if audited := e.WsScalar(t, `SELECT after::text FROM audit_log WHERE entity_type = 'privacy_request_proof'`); strings.Contains(audited, "proof.test") {
		t.Fatalf("the proof's audit image %s holds the requester's address", audited)
	}
}

func TestClosingARequestFiledByHandNeedsTheIdentityCheck(t *testing.T) {
	e := setupProofs(t)
	contact := e.SeedContact(t, "Unchecked Subject", nil)
	request := e.openRequest(t, "erasure", contact.String())
	r := httptest.NewRequest(http.MethodPatch, "/v1/data-subject-requests/"+request.String(),
		strings.NewReader(`{"status":"fulfilled","resolution":"erased"}`)).WithContext(e.Admin())
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	e.handlers.UpdateDataSubjectRequest(w, r, openapi_types.UUID(request))

	if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "identity_check") {
		t.Fatalf("a close without an identity check → %d %s, want 422 naming identity_check", w.Code, w.Body)
	}
	if name := e.WsScalar(t, `SELECT full_name FROM contact WHERE id = $1`, contact); name != "Unchecked Subject" {
		t.Fatalf("the refused close erased the contact (name %q)", name)
	}
	if n := e.proofCount(t, request); n != 0 {
		t.Fatalf("%d proofs after a refused close", n)
	}
}

// answerProofAt moves a proof's answer date, and its retention date by the
// writer's own rule. The sweep then meets ages a test cannot wait for.
func (e proofEnv) answerProofAt(t *testing.T, request ids.UUID, answered time.Time) {
	t.Helper()
	e.WsExec(t, `UPDATE privacy_request_proof SET answered_at = $2, keep_until = $3 WHERE request_id = $1`,
		request, answered, consent.ProofKeepUntil(answered))
}

func (e proofEnv) closeAccess(t *testing.T, subjectRef string) ids.UUID {
	t.Helper()
	request := e.openRequest(t, "access", subjectRef)
	e.patch(t, request, `{"status":"fulfilled","resolution":"package sent","identity_check":"address_on_file"}`)
	return request
}

func (e proofEnv) expire(t *testing.T, now time.Time) {
	t.Helper()
	worker := &requestProofExpiryWorker{
		requests: e.store, identity: identity.NewService(e.Pool), now: func() time.Time { return now },
		log: slog.New(slog.DiscardHandler),
	}
	if err := worker.Work(context.Background(), nil); err != nil {
		t.Fatalf("expiry pass: %v", err)
	}
}

func TestTheJanuaryRunDeletesProofsThreeYearsAfterTheirYearAndKeepsTheRest(t *testing.T) {
	e := setupProofs(t)
	due := e.closeAccess(t, "due@proof.test")
	younger := e.closeAccess(t, "younger@proof.test")
	disputed := e.closeAccess(t, "disputed@proof.test")
	e.answerProofAt(t, due, time.Date(2026, 12, 31, 9, 0, 0, 0, time.UTC))
	e.answerProofAt(t, younger, time.Date(2027, 1, 1, 9, 0, 0, 0, time.UTC))
	e.answerProofAt(t, disputed, time.Date(2026, 6, 1, 9, 0, 0, 0, time.UTC))
	e.patch(t, disputed, `{"dispute_open":true}`)

	e.expire(t, time.Date(2029, 12, 31, 23, 0, 0, 0, time.UTC))
	if n := e.proofCount(t, due); n != 1 {
		t.Fatal("a proof went before its third year had ended")
	}
	e.expire(t, time.Date(2030, 2, 1, 0, 0, 0, 0, time.UTC))
	if n := e.proofCount(t, due); n != 1 {
		t.Fatal("a run outside January deleted a proof")
	}
	e.expire(t, time.Date(2030, 1, 2, 3, 0, 0, 0, time.UTC))
	if n := e.proofCount(t, due); n != 0 {
		t.Fatal("the January run kept a proof three years past the end of its year")
	}
	if n := e.proofCount(t, younger); n != 1 {
		t.Fatal("the January run deleted a proof answered a day later, in the next year")
	}
	if n := e.proofCount(t, disputed); n != 1 {
		t.Fatal("the January run deleted a proof whose request is under dispute")
	}

	e.patch(t, disputed, `{"dispute_open":false}`)
	e.expire(t, time.Date(2030, 1, 20, 3, 0, 0, 0, time.UTC))
	if n := e.proofCount(t, disputed); n != 0 {
		t.Fatal("a proof outlived the dispute that held it")
	}
}

func TestAnExpiredProofTakesTheRequestersReferenceFromTheRequestAndItsAuditImage(t *testing.T) {
	e := setupProofs(t)
	const reference = "expiring.requester@proof.test"
	request := e.closeAccess(t, reference)
	e.answerProofAt(t, request, time.Date(2025, 3, 1, 9, 0, 0, 0, time.UTC))

	entity, id := "data_subject_request", request
	readTrail := func() string {
		page, err := privacy.ListAuditLog(e.Admin(), e.DB(), privacy.AuditFilter{EntityType: &entity, EntityID: &id})
		if err != nil {
			t.Fatal(err)
		}
		var trail strings.Builder
		for _, entry := range page.Entries {
			trail.Write(entry.Before)
			trail.Write(entry.After)
		}
		return trail.String()
	}
	if !strings.Contains(readTrail(), reference) {
		t.Fatal("the creation image never held the reference, so its absence below would prove nothing")
	}

	e.expire(t, time.Date(2029, 1, 3, 3, 0, 0, 0, time.UTC))

	if got := e.WsScalar(t, `SELECT subject_ref FROM data_subject_request WHERE id = $1`, request); got != "erased+"+request.String() {
		t.Fatalf("the expired request still names %q", got)
	}
	if trail := readTrail(); strings.Contains(trail, reference) {
		t.Fatalf("the audit trail still shows the reference: %s", trail)
	}
}

func TestTheProofBackfillRebuildsWhatTheRecordsHoldAndReplaysSafely(t *testing.T) {
	e := setupProofs(t)
	contact := e.SeedContact(t, "Earlier Subject", nil)
	erasure := e.openRequest(t, "erasure", contact.String())
	e.patch(t, erasure, `{"status":"fulfilled","resolution":"erased","identity_check":"id_document"}`)
	access := e.closeAccess(t, "earlier@proof.test")
	erased := e.proofCounts(t, erasure)
	// The state an installation was in before proofs were written.
	e.WsExec(t, `DELETE FROM privacy_request_proof`)

	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	step := requestProofDataTask().Steps[0]
	var estimated int64
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		estimated, _, err = step.Estimate(ctx, tx, 1000)
		return err
	}); err != nil || estimated != 2 {
		t.Fatalf("estimate = %d, %v, want 2", estimated, err)
	}
	task := requestProofDataTask()
	task.Steps[0].Limit = 1
	runMachineAddressTask(ctx, t, e.Env, task)

	if got := e.proofCounts(t, erasure); !maps.Equal(got, erased) {
		t.Fatalf("backfilled counts %v, want the tombstone's %v", got, erased)
	}
	got := e.WsScalar(t, `SELECT string_agg(concat_ws('|', request_kind, identity_check, erased_counts_known::text,
		coalesce(requester_email, '-'), completed_by_id), ',' ORDER BY request_kind) FROM privacy_request_proof`)
	admin := "human:" + e.AdminUser.String()
	if want := "access|unknown|true|earlier@proof.test|" + admin + ",erasure|unknown|true|-|" + admin; got != want {
		t.Fatalf("backfilled proofs = %s, want %s", got, want)
	}
	if n := e.proofCount(t, access); n != 1 {
		t.Fatalf("%d proofs for the access request, want 1", n)
	}

	audits := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'privacy_request_proof'`)
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		out, err := step.Batch(ctx, tx, datatask.BatchIn{Limit: 10, RunID: ids.NewV7()})
		if err == nil && (out.Changed != 0 || out.Scanned != 2) {
			t.Errorf("replay changed %d of %d, want 0 of 2", out.Changed, out.Scanned)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if again := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'privacy_request_proof'`); again != audits {
		t.Fatalf("a replay raised the proof audit rows from %d to %d", audits, again)
	}
}

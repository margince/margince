// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"encoding/json"
	"errors"
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
	"github.com/margince/margince/backend/internal/shared/kernel/erasureaccount"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// proofEnv is one installation with the consent handlers the server builds.
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
		handlers: newConsentHandlers(e.Pool),
		store:    consent.NewStore(e.DB()).WithInstallationZone(identity.ZoneAppliedTx),
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

func patchWith(ctx context.Context, handlers consent.Handlers, id ids.UUID, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPatch, "/v1/data-subject-requests/"+id.String(),
		strings.NewReader(body)).WithContext(ctx)
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handlers.UpdateDataSubjectRequest(w, r, openapi_types.UUID(id))
	return w
}

func (e proofEnv) patch(t *testing.T, id ids.UUID, body string) {
	t.Helper()
	if w := patchWith(e.Admin(), e.handlers, id, body); w.Code != http.StatusOK {
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

// tombstone reads the account the erasure engine certified for request, so
// the proof can be held to the engine's own numbers.
func (e proofEnv) tombstone(t *testing.T, request ids.UUID) erasureaccount.Account {
	t.Helper()
	var account erasureaccount.Account
	if err := json.Unmarshal([]byte(e.WsScalar(t, `
		SELECT evidence::text FROM audit_log
		 WHERE entity_type = 'contact' AND action = 'erase' AND evidence->>'reason' = $1`,
		"dsr:"+request.String())), &account); err != nil {
		t.Fatal(err)
	}
	return account
}

func (e proofEnv) seedNoteFor(t *testing.T, contact ids.UUID, subject, body string) {
	t.Helper()
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "note", Subject: &subject, Body: &body, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	}); err != nil {
		t.Fatal(err)
	}
}

// erasureTrail counts the erase tombstones of request and the erase events on
// contact.
func (e proofEnv) erasureTrail(t *testing.T, contact, request ids.UUID) (tombstones, events int) {
	t.Helper()
	tombstones = e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'contact' AND action = 'erase'
		AND evidence->>'reason' = $1`, "dsr:"+request.String())
	events = e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'retention.applied'
		AND envelope->'entity'->>'id' = $1::text`, contact)
	return tombstones, events
}

// installationZone is the zone the proof writer counts in.
func (e proofEnv) installationZone(t *testing.T) *time.Location {
	t.Helper()
	var zone *time.Location
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		var err error
		zone, err = identity.ZoneAppliedTx(e.Admin(), tx)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return zone
}

func TestFulfillingAnErasureWritesOneProofWithTheErasuresOwnAccount(t *testing.T) {
	e := setupProofs(t)
	contact := e.SeedContact(t, "Henrike Proofsubject", nil)
	subject, body := "Loeschantrag Henrike", "Bitte alles loeschen, Ihre Henrike"
	e.seedNoteFor(t, contact, subject, body)
	request := e.openRequest(t, "erasure", contact.String())
	closing := `{"status":"fulfilled","resolution":"erased","identity_check":"id_document","requester_email":"Henrike@Proof.test"}`

	e.patch(t, request, closing)
	first := e.tombstone(t, request)
	// The erased contact refuses a new link. So the later text lands on the
	// timeline it already has, as an edit of the note would write it.
	const later = "Noch eine Nachricht nach der Loeschung"
	e.WsExec(t, `UPDATE activity SET body = $2 WHERE id IN (
		SELECT activity_id FROM activity_link WHERE contact_id = $1)`, contact, later)
	e.patch(t, request, closing)

	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE body = $1`, later); n != 0 {
		t.Fatalf("the replay left %d note(s) written after the first erasure", n)
	}
	if n := e.proofCount(t, request); n != 1 {
		t.Fatalf("%d proofs after a fulfilment and its replay, want 1", n)
	}
	if again := e.tombstone(t, request); !maps.Equal(again.Rows, first.Rows) {
		t.Fatalf("the replay changed the certified account from %v to %v", first.Rows, again.Rows)
	}
	if tombstones, events := e.erasureTrail(t, contact, request); tombstones != 1 || events != 1 {
		t.Fatalf("%d erase tombstones and %d events after a replay, want 1 and 1", tombstones, events)
	}
	counts := e.proofCounts(t, request)
	if want := e.tombstone(t, request).Rows; !maps.Equal(counts, want) {
		t.Fatalf("proof counts %v, want the erasure's own %v", counts, want)
	}
	if counts["activity.updated"] < 1 || counts["contact.updated"] < 1 {
		t.Fatalf("proof counts %v, want the note and the contact among them", counts)
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

// failingAfterErase erases for real, then fails: what FulfilErasure sees when
// the erasure committed and the close cannot finish.
type failingAfterErase struct{ inner consent.Eraser }

var errClosingDied = errors.New("the close died after the erase")

func (f failingAfterErase) EraseContactCounted(ctx context.Context, contact ids.UUID, reason string) (erasureaccount.Account, error) {
	if _, err := f.inner.EraseContactCounted(ctx, contact, reason); err != nil {
		return erasureaccount.Account{}, err
	}
	return erasureaccount.Account{}, errClosingDied
}

func TestARetriedCloseStoresTheFirstErasuresAccount(t *testing.T) {
	e := setupProofs(t)
	contact := e.SeedContact(t, "Retried Subject", nil)
	e.seedNoteFor(t, contact, "Bitte loeschen", "Bitte loeschen Sie alles")
	request := e.openRequest(t, "erasure", contact.String())
	closing := `{"status":"fulfilled","resolution":"erased","identity_check":"id_document"}`

	dying := consent.NewHandlers(e.DB()).WithEraser(failingAfterErase{inner: privacy.NewEraser(e.DB())})
	if w := patchWith(e.Admin(), dying, request, closing); w.Code == http.StatusOK {
		t.Fatalf("the dying close answered %d", w.Code)
	}
	if n := e.proofCount(t, request); n != 0 {
		t.Fatalf("%d proofs after a close that died", n)
	}
	e.patch(t, request, closing)

	counts := e.proofCounts(t, request)
	if want := e.tombstone(t, request).Rows; !maps.Equal(counts, want) || counts["activity.updated"] < 1 {
		t.Fatalf("the retried proof counts %v, want the first erasure's %v", counts, want)
	}
	if tombstones, _ := e.erasureTrail(t, contact, request); tombstones != 1 {
		t.Fatalf("%d erase tombstones after a retry, want 1", tombstones)
	}
}

func TestARequestWhoseIdentityWasNotCheckedCannotBeFulfilled(t *testing.T) {
	e := setupProofs(t)
	contact := e.SeedContact(t, "Unverified Subject", nil)
	for kind, subjectRef := range map[string]string{"access": "unverified@proof.test", "erasure": contact.String()} {
		request := e.openRequest(t, kind, subjectRef)
		w := patchWith(e.Admin(), e.handlers, request,
			`{"status":"fulfilled","resolution":"done","identity_check":"not_verified"}`)
		if w.Code != http.StatusUnprocessableEntity || !strings.Contains(w.Body.String(), "identity_check") {
			t.Fatalf("%s fulfilled with not_verified → %d %s, want 422 naming identity_check", kind, w.Code, w.Body)
		}
		if status := e.WsScalar(t, `SELECT status FROM data_subject_request WHERE id = $1`, request); status != "open" {
			t.Fatalf("the refused %s request is %s", kind, status)
		}
		if n := e.proofCount(t, request); n != 0 {
			t.Fatalf("%d proofs after a refused %s close", n, kind)
		}
	}
	if name := e.WsScalar(t, `SELECT full_name FROM contact WHERE id = $1`, contact); name != "Unverified Subject" {
		t.Fatalf("the refused erasure erased the contact (name %q)", name)
	}
}

func TestARejectedRequestKeepsAProofThatErasedNothing(t *testing.T) {
	e := setupProofs(t)
	request := e.openRequest(t, "access", "rejected@proof.test")
	e.patch(t, request, `{"status":"rejected","resolution":"not the subject","identity_check":"not_verified"}`)

	got := e.WsScalar(t, `SELECT concat_ws('|', result, identity_check, request_kind, requester_email,
		erased_counts::text, cardinality(erased_uncounted)::text, erased_counts_known::text, completed_by_id)
		FROM privacy_request_proof WHERE request_id = $1`, request)
	if want := "rejected|not_verified|access|rejected@proof.test|{}|0|true|human:" + e.AdminUser.String(); got != want {
		t.Fatalf("rejected proof = %s, want %s", got, want)
	}
}

func TestClosingARequestFiledByHandNeedsTheIdentityCheck(t *testing.T) {
	e := setupProofs(t)
	contact := e.SeedContact(t, "Unchecked Subject", nil)
	request := e.openRequest(t, "erasure", contact.String())
	w := patchWith(e.Admin(), e.handlers, request, `{"status":"fulfilled","resolution":"erased"}`)
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

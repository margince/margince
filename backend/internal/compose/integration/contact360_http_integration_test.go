// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The contact page's three HTTP surfaces, driven through the real handlers.
//
// The service tests beside this one prove the assembly; these prove the wire:
// the status a refusal answers with, the shape the client parses, and the
// view-ack's monotonicity, which is the one thing a GET must never do for
// itself.

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/contact360"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func contactHandlers(e *Env) contact360.Handlers {
	return contact360.NewHandlers(
		contact360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects, consent.NewStore(e.DB()),
			comms.NewStore(e.DB(), time.Now, activities.NewStore(e.DB())), ai.NewFeedbackStore(e.DB()), func() time.Time { return roomFixedNow }))
}

// call drives one handler and returns the status and body.
func call(ctx context.Context, method, path string,
	run func(http.ResponseWriter, *http.Request),
) (int, []byte) {
	rec := httptest.NewRecorder()
	run(rec, httptest.NewRequest(method, path, nil).WithContext(ctx))
	return rec.Code, rec.Body.Bytes()
}

// A contact outside row scope is a 404 on every surface. An empty 200 would
// confirm the record exists and only its contents are withheld. Capture
// privacy is what puts a colleague's contact out of scope.
func TestContact360SurfacesAllRefuseAForeignContactWith404(t *testing.T) {
	e := Setup(t)
	theirs := e.SeedContact(t, "Their Contact", &e.Rep3)
	e.MakeCapturePrivate(t, "contact", theirs, e.Rep3)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, roomPerms)
	h := contactHandlers(e)
	id := crmcontracts.Id(theirs)

	for name, run := range map[string]func(http.ResponseWriter, *http.Request){
		"the composite read": func(w http.ResponseWriter, r *http.Request) {
			h.GetContact360(w, r, id, crmcontracts.GetContact360Params{})
		},
		"the profile-field sidecar": func(w http.ResponseWriter, r *http.Request) {
			h.GetContactProfileFields(w, r, id)
		},
		"the view acknowledgement": func(w http.ResponseWriter, r *http.Request) {
			h.AcknowledgeContactView(w, r, id)
		},
	} {
		if code, _ := call(rep, http.MethodGet, "/v1/contacts/"+theirs.String(), run); code != http.StatusNotFound {
			t.Errorf("%s → %d, want 404", name, code)
		}
	}
}

// The composite read answers one JSON object the client can parse, with the
// root record present and the omitted list always an array — never null, which
// a client would have to special-case.
func TestGetContact360AnswersOneParseableComposite(t *testing.T) {
	e := Setup(t)
	mine := e.SeedContact(t, "Anna Weber", &e.Rep1)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, roomPerms)
	h := contactHandlers(e)

	code, body := call(rep, http.MethodGet, "/v1/contacts/"+mine.String()+"/360",
		func(w http.ResponseWriter, r *http.Request) {
			h.GetContact360(w, r, crmcontracts.Id(mine), crmcontracts.GetContact360Params{})
		})
	if code != http.StatusOK {
		t.Fatalf("360 → %d, want 200", code)
	}
	var page crmcontracts.Contact360
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("the composite did not parse: %v", err)
	}
	if page.Contact.Id != crmcontracts.Id(mine) {
		t.Error("the page came back about a different record")
	}
	if page.SectionsOmitted == nil {
		t.Error("sections_omitted is null; a client would have to special-case it")
	}
	if page.AsOf.IsZero() {
		t.Error("as_of is unset, so nothing says which moment the sections describe")
	}
}

// The baseline moves forward ONLY on the acknowledgement. A GET that advanced
// it would destroy the very "what changed" answer the caller opened the page
// to read, and make a prefetch indistinguishable from a visit.
func TestTheViewBaselineMovesOnlyOnTheAcknowledgement(t *testing.T) {
	e := Setup(t)
	mine := e.SeedContact(t, "Anna Weber", &e.Rep1)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, roomPerms)
	h := contactHandlers(e)

	baseline := func() int {
		var n int
		if err := OwnerConn(t).QueryRow(context.Background(),
			`SELECT count(*) FROM user_record_view WHERE entity_type = 'contact' AND entity_id = $1`,
			mine).Scan(&n); err != nil {
			t.Fatalf("reading the baseline: %v", err)
		}
		return n
	}

	if code, _ := call(rep, http.MethodGet, "/v1/contacts/"+mine.String()+"/360",
		func(w http.ResponseWriter, r *http.Request) {
			h.GetContact360(w, r, crmcontracts.Id(mine), crmcontracts.GetContact360Params{})
		}); code != http.StatusOK {
		t.Fatalf("360 → %d", code)
	}
	if baseline() != 0 {
		t.Fatal("reading the page moved the visit baseline; the next read's since-last-visit is now empty")
	}

	code, body := call(rep, http.MethodPost, "/v1/contacts/"+mine.String()+"/view-ack",
		func(w http.ResponseWriter, r *http.Request) { h.AcknowledgeContactView(w, r, crmcontracts.Id(mine)) })
	if code != http.StatusOK {
		t.Fatalf("view-ack → %d, want 200", code)
	}
	var ack crmcontracts.RecordViewAck
	if err := json.Unmarshal(body, &ack); err != nil {
		t.Fatalf("the acknowledgement did not parse: %v", err)
	}
	if baseline() != 1 {
		t.Error("the acknowledgement did not record a visit")
	}

	// Twice is once: the upsert is monotonic, so a second tab cannot rewind a
	// newer mark or accumulate rows.
	if code, _ := call(rep, http.MethodPost, "/v1/contacts/"+mine.String()+"/view-ack",
		func(w http.ResponseWriter, r *http.Request) { h.AcknowledgeContactView(w, r, crmcontracts.Id(mine)) }); code != http.StatusOK {
		t.Fatalf("second view-ack → %d", code)
	}
	if baseline() != 1 {
		t.Error("a second acknowledgement wrote a second baseline row")
	}
}

// The sidecar answers {data: []} — an empty list when nothing has been
// enriched, which is a different fact from the endpoint refusing. The list is
// never null: a client would have to special-case that.
func TestGetContactProfileFieldsAnswersAnArrayWhenNothingIsEnriched(t *testing.T) {
	e := Setup(t)
	mine := e.SeedContact(t, "Anna Weber", &e.Rep1)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, roomPerms)
	h := contactHandlers(e)

	code, body := call(rep, http.MethodGet, "/v1/contacts/"+mine.String()+"/profile-fields",
		func(w http.ResponseWriter, r *http.Request) { h.GetContactProfileFields(w, r, crmcontracts.Id(mine)) })
	if code != http.StatusOK {
		t.Fatalf("profile-fields → %d, want 200", code)
	}
	var out struct {
		Data []crmcontracts.ContactProfileField `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("the sidecar did not parse: %v", err)
	}
	if out.Data == nil {
		t.Error("the sidecar answered a null list rather than an empty one")
	}
}

// A section this reader may not see comes back nil, exactly like a section that
// is genuinely empty, and the difference is recorded in sections_omitted. A
// moment rule that reads nil as "there is nothing here" therefore tells a
// reader without the grant that nothing is scheduled and nobody is waiting on a
// reply — confident statements about data the page was never allowed to look at.
//
// The unit tests for that guard hand-build sections_omitted. This one drives
// the real assembler with a real principal, so it also proves the guard is
// keyed to the omission shape assemble.go actually produces rather than to one
// a test invented.
func TestAMomentDoesNotClaimAbsenceForASectionTheReaderCouldNotSee(t *testing.T) {
	e := Setup(t)
	mine := e.SeedContact(t, "Anna Weber", &e.Rep1)
	h := contactHandlers(e)

	read := func(ctx context.Context) crmcontracts.Contact360 {
		t.Helper()
		code, body := call(ctx, http.MethodGet, "/v1/contacts/"+mine.String()+"/360",
			func(w http.ResponseWriter, r *http.Request) {
				h.GetContact360(w, r, crmcontracts.Id(mine), crmcontracts.GetContact360Params{})
			})
		if code != http.StatusOK {
			t.Fatalf("360 → %d, want 200", code)
		}
		var page crmcontracts.Contact360
		if err := json.Unmarshal(body, &page); err != nil {
			t.Fatalf("the composite did not parse: %v", err)
		}
		return page
	}

	// The same record, read by someone who may not see activities.
	blind := roomPerms
	blind.Objects = map[string]principal.ObjectGrant{}
	maps.Copy(blind.Objects, roomPerms.Objects)
	delete(blind.Objects, "activity")

	page := read(e.As(e.Rep1, []ids.UUID{e.Team1}, blind))
	if len(page.SectionsOmitted) == 0 {
		t.Fatal("a reader without the activity grant must be TOLD which sections were withheld")
	}
	if page.Moment == nil {
		t.Fatal("the page still opens on a moment")
	}
	// Whatever the ladder chose, it must not be a verdict about the sections
	// this reader was refused.
	for _, claim := range []string{"nobody waiting on a reply", "nothing owed"} {
		if strings.Contains(page.Moment.WhyNow, claim) {
			t.Errorf("the moment says %q to a reader shown %d withheld section(s): %q",
				claim, len(page.SectionsOmitted), page.Moment.WhyNow)
		}
	}
}

// A deal seat says whether its deal is still open and whether the seat itself
// has ended, so the composer offers only a deal a follow-up can rest on.
func TestADealSeatSaysWhetherItsDealIsOpen(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	pipeline, open, _ := DealFixture(t, e)
	mine := e.SeedContact(t, "Anna Weber", &e.Rep1)
	live := e.SeedDeal(t, "Still talking", pipeline, open, &e.Rep1)
	closed := e.SeedDeal(t, "Went elsewhere", pipeline, open, &e.Rep1)
	left := e.SeedDeal(t, "They moved on", pipeline, open, &e.Rep1)
	reason, reasonCode := "Went with a competitor", "competitor"
	if _, err := e.Deals.AdvanceDeal(e.Admin(), ids.From[ids.DealKind](closed), deals.AdvanceDealInput{
		ToStageID: lostStageOf(t, e), LostReason: &reason, LostReasonCode: &reasonCode,
	}); err != nil {
		t.Fatalf("losing the deal: %v", err)
	}
	for _, deal := range []ids.UUID{live, closed, left} {
		var ended *time.Time
		if deal == left {
			ended = &roomFixedNow
		}
		if _, err := owner.Exec(context.Background(),
			`INSERT INTO relationship (kind, contact_id, deal_id, ended_at, source, captured_by)
			 VALUES ('deal_stakeholder', $1, $2, $3, 'manual', 'human:x')`, mine, deal, ended); err != nil {
			t.Fatal(err)
		}
	}

	code, body := call(e.As(e.Rep1, []ids.UUID{e.Team1}, roomPerms), http.MethodGet,
		"/v1/contacts/"+mine.String()+"/360", func(w http.ResponseWriter, r *http.Request) {
			contactHandlers(e).GetContact360(w, r, crmcontracts.Id(mine), crmcontracts.GetContact360Params{})
		})
	if code != http.StatusOK {
		t.Fatalf("360 → %d, want 200", code)
	}
	var page crmcontracts.Contact360
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("the composite did not parse: %v", err)
	}
	if page.DealRoles == nil {
		t.Fatal("deal_roles is absent for a reader with the deal grant")
	}
	want := map[ids.UUID]crmcontracts.Contact360DealRoleDealStatus{
		live: crmcontracts.Contact360DealRoleDealStatusOpen, closed: crmcontracts.Contact360DealRoleDealStatusLost,
		left: crmcontracts.Contact360DealRoleDealStatusOpen,
	}
	for _, seat := range page.DealRoles.Data {
		deal := ids.UUID(seat.DealId)
		if seat.DealStatus == nil || *seat.DealStatus != want[deal] {
			t.Errorf("deal %s: status %v, want %q", deal, seat.DealStatus, want[deal])
		}
		if seat.SeatCurrent == nil || *seat.SeatCurrent != (deal != left) {
			t.Errorf("deal %s: seat_current %v, want %v", deal, seat.SeatCurrent, deal != left)
		}
	}
	if len(page.DealRoles.Data) != len(want) {
		t.Errorf("deal_roles has %d seats, want %d", len(page.DealRoles.Data), len(want))
	}
}

// lostStageOf is the default pipeline's lost stage.
func lostStageOf(t *testing.T, e *Env) ids.StageID {
	t.Helper()
	p, err := e.Deals.DefaultPipeline(e.Admin())
	if err != nil {
		t.Fatal(err)
	}
	for _, st := range *p.Stages {
		if st.Semantic == "lost" {
			return ids.From[ids.StageKind](ids.UUID(st.Id))
		}
	}
	t.Fatal("the default pipeline has no lost stage")
	return ids.StageID{}
}

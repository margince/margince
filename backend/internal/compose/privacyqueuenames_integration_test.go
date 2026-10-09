// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The privacy queues name their records through the production wiring, so a
// name shows only to a reader who may see the record.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ownScopeOfficer is Rep1 on the privacy queues with an own-scope CRM view, so
// a colleague's capture-private contact is what it may not open.
func ownScopeOfficer(e *integration.Env) context.Context {
	return e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects: map[string]principal.ObjectGrant{
			"contact":         {Read: true},
			"lead":            {Read: true},
			"privacy_request": {Read: true, Update: true},
		},
		RowScope: principal.RowScopeOwn,
	})
}

// listOverHTTP drives one consent handler and decodes its page.
func listOverHTTP[T any](ctx context.Context, t *testing.T, serve func(http.ResponseWriter, *http.Request)) []T {
	t.Helper()
	rec := httptest.NewRecorder()
	serve(rec, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx))
	if rec.Code != http.StatusOK {
		t.Fatalf("listing → %d: %s", rec.Code, rec.Body.String())
	}
	var page struct {
		Data []T `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &page); err != nil {
		t.Fatalf("decoding the page: %v", err)
	}
	return page.Data
}

func TestTheNoticeQueueNamesOnlyContactsItsReaderMayOpen(t *testing.T) {
	e := integration.Setup(t)
	due := time.Now().Add(72 * time.Hour)
	own := homeNotice(t, e, e.Rep1, false, due)
	foreign := homeNotice(t, e, e.Rep3, true, due)

	h := newConsentHandlers(e.Pool)
	rows := listOverHTTP[crmcontracts.NoticeCase](ownScopeOfficer(e), t, func(w http.ResponseWriter, r *http.Request) {
		h.ListNoticeCases(w, r, crmcontracts.ListNoticeCasesParams{})
	})
	byID := map[ids.UUID]crmcontracts.NoticeCase{}
	for _, row := range rows {
		byID[ids.UUID(row.Id)] = row
	}
	if got := byID[own.ID].ContactName; got == nil || *got != "Agenda contact" {
		t.Errorf("the reader's own contact is named %v, want %q", got, "Agenda contact")
	}
	if got := byID[foreign.ID].ContactName; got != nil {
		t.Errorf("a contact outside the reader's scope is named %q: the queue may reach the duty, "+
			"never the name", *got)
	}
	for owner, id := range map[ids.UUID]ids.UUID{e.Rep1: own.ID, e.Rep3: foreign.ID} {
		evidence := byID[id].Acquisition
		if evidence == nil {
			t.Fatalf("duty %s carries no acquisition, so its deadline reads as authoritative", id)
		}
		if evidence.Kind != contacts.AcquiredPurchasedOrImported || evidence.CapturedBy != "human:"+owner.String() {
			t.Errorf("duty %s rests on %+v, want a purchase recorded by %s", id, evidence, owner)
		}
		if evidence.CapturedByName == nil || *evidence.CapturedByName != "Rep" {
			t.Errorf("the seat that recorded duty %s is named %v, want its display name", id, evidence.CapturedByName)
		}
		if evidence.OccurredAt != nil || evidence.CapturedAt.IsZero() {
			t.Errorf("an undated acquisition answers occurred %v, captured %v; want null and the write time",
				evidence.OccurredAt, evidence.CapturedAt)
		}
	}
}

func TestAnArchivedMembersAcquisitionNamesNobody(t *testing.T) {
	e := integration.Setup(t)
	duty := homeNotice(t, e, e.Rep3, false, time.Now().Add(72*time.Hour))
	args := []any{e.Rep3}
	e.WsExec(t, storekit.SQLf("UPDATE app_user SET archived_at = now() WHERE id = $%d", len(args)), args...)

	h := newConsentHandlers(e.Pool)
	rows := listOverHTTP[crmcontracts.NoticeCase](e.Admin(), t, func(w http.ResponseWriter, r *http.Request) {
		h.ListNoticeCases(w, r, crmcontracts.ListNoticeCasesParams{})
	})
	for _, row := range rows {
		if ids.UUID(row.Id) != duty.ID {
			continue
		}
		if row.Acquisition == nil || row.Acquisition.CapturedBy != "human:"+e.Rep3.String() {
			t.Fatalf("the duty rests on %+v, want the archived seat's principal kept", row.Acquisition)
		}
		if row.Acquisition.CapturedByName != nil {
			t.Errorf("an archived seat is named %q, want null as SeatNames answers", *row.Acquisition.CapturedByName)
		}
		return
	}
	t.Fatalf("duty %s is not in the queue", duty.ID)
}

func TestTheNoticeAgendaCarriesTheAcquisitionToTheWorklist(t *testing.T) {
	e := integration.Setup(t)
	duty := homeNotice(t, e, e.AdminUser, false, homeReadTime.Add(-time.Hour))

	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return homeReadTime })
	day, err := feed.Worklist(e.Admin(), "mine", "all", ids.Nil, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range day.Queue {
		if row.Id != duty.ID.String() {
			continue
		}
		if row.Acquisition == nil || row.Acquisition.Kind != contacts.AcquiredPurchasedOrImported {
			t.Fatalf("the duty's worklist row carries acquisition %+v, want the purchase it rests on", row.Acquisition)
		}
		return
	}
	t.Fatalf("the reader's duty %s is not on the worklist", duty.ID)
}

func TestTheSubjectQueueNamesOnlyRecordsItsReaderMayOpen(t *testing.T) {
	e := integration.Setup(t)
	contactAs := func(owner ids.UUID, name string) string {
		t.Helper()
		contact, err := e.Contacts.CreateContact(e.As(owner, nil, integration.AdminPerms),
			contacts.CreateContactInput{FullName: name, Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		return ids.UUID(contact.Id).String()
	}
	ownName, leadName, rep1 := "Own subject", "Lead subject", ids.From[ids.UserKind](e.Rep1)
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: &leadName, Source: "manual", OwnerID: &rep1})
	if err != nil {
		t.Fatal(err)
	}
	// A nil label and kind mean withheld or unresolved.
	type named struct {
		label *string
		kind  crmcontracts.DataSubjectRequestSubjectKind
	}
	refs := map[string]named{
		contactAs(e.Rep1, ownName):                                    {&ownName, crmcontracts.DataSubjectRequestSubjectKindContact},
		homeNotice(t, e, e.Rep3, true, time.Now()).ContactID.String(): {},
		ids.UUID(lead.Id).String():                                    {&leadName, crmcontracts.DataSubjectRequestSubjectKindLead},
		"someone@example.test":                                        {},
	}
	store := consent.NewStore(e.DB())
	for ref := range refs {
		if _, err := store.CreateDSR(e.Admin(), consent.CreateDSRInput{
			Kind: "access", SubjectRef: ref, DueAt: time.Now().Add(30 * 24 * time.Hour),
		}); err != nil {
			t.Fatalf("filing a request about %s: %v", ref, err)
		}
	}

	h := newConsentHandlers(e.Pool)
	rows := listOverHTTP[crmcontracts.DataSubjectRequest](ownScopeOfficer(e), t, func(w http.ResponseWriter, r *http.Request) {
		h.ListDataSubjectRequests(w, r, crmcontracts.ListDataSubjectRequestsParams{})
	})
	if len(rows) != len(refs) {
		t.Fatalf("the queue holds %d requests, want %d", len(rows), len(refs))
	}
	for _, row := range rows {
		want, got := refs[row.SubjectRef], row.SubjectLabel
		if (want.label == nil) != (got == nil) || (want.label != nil && *want.label != *got) {
			t.Errorf("the request about %q is labelled %v, want %v", row.SubjectRef, got, want.label)
		}
		if gotKind := row.SubjectKind; (want.kind == "") != (gotKind == nil) || (gotKind != nil && *gotKind != want.kind) {
			t.Errorf("the request about %q resolved to kind %v, want %q (empty means null)", row.SubjectRef, gotKind, want.kind)
		}
	}
}

func TestAnOfficerWithoutTheContactGrantSeesNoNames(t *testing.T) {
	e := integration.Setup(t)
	homeNotice(t, e, e.Rep1, false, time.Now().Add(72*time.Hour))
	officer := e.As(e.Rep1, nil, principal.Permissions{
		RoleKeys: []string{"privacy"},
		Objects:  map[string]principal.ObjectGrant{"privacy_request": {Read: true}},
		RowScope: principal.RowScopeAll,
	})

	h := newConsentHandlers(e.Pool)
	rows := listOverHTTP[crmcontracts.NoticeCase](officer, t, func(w http.ResponseWriter, r *http.Request) {
		h.ListNoticeCases(w, r, crmcontracts.ListNoticeCasesParams{})
	})
	if len(rows) != 1 {
		t.Fatalf("the queue holds %d duties, want the one seeded", len(rows))
	}
	if rows[0].ContactName != nil {
		t.Errorf("an officer who may not read contacts is told %q", *rows[0].ContactName)
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// offers_email on every activity read, against the To that reply-recipient fills.
// Each case reads both off the same row, so the verb and the address cannot part.

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/contact360"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const ownDomain = "ours.test"

func (s *standingEnv) reader() context.Context {
	return s.As(s.Rep1, []ids.UUID{s.Team1}, roomPerms)
}

func serve(t *testing.T, ctx context.Context, call func(http.ResponseWriter, *http.Request)) (int, []byte) {
	t.Helper()
	rec := httptest.NewRecorder()
	call(rec, httptest.NewRequest(http.MethodGet, "/v1/x", nil).WithContext(ctx))
	return rec.Code, rec.Body.Bytes()
}

func readBody[T crmcontracts.Activity | crmcontracts.ReplyRecipient | crmcontracts.Contact360 | activityPage](t *testing.T, body []byte) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	return out
}

type activityPage struct{ Data []crmcontracts.Activity }

// offeredOnEveryRead is the row's offers_email off the list and the single read,
// failing when the two disagree, beside the To reply-recipient fills.
func (s *standingEnv) offeredOnEveryRead(t *testing.T, row ids.UUID) (offers bool, to string) {
	t.Helper()
	ctx, h := s.reader(), s.replyHandlers()
	code, body := serve(t, ctx, func(w http.ResponseWriter, r *http.Request) {
		h.ListActivities(w, r, crmcontracts.ListActivitiesParams{})
	})
	if code != http.StatusOK {
		t.Fatalf("GET /activities answered %d", code)
	}
	page := readBody[activityPage](t, body)
	listed, found := false, false
	for _, a := range page.Data {
		if ids.UUID(a.Id) == row {
			listed, found = a.OffersEmail, true
		}
	}
	if !found {
		t.Fatal("the row is missing from GET /activities")
	}
	code, body = serve(t, ctx, func(w http.ResponseWriter, r *http.Request) { h.GetActivity(w, r, crmcontracts.Id(row)) })
	if code != http.StatusOK {
		t.Fatalf("GET /activities/{id} answered %d", code)
	}
	one := readBody[crmcontracts.Activity](t, body)
	if one.OffersEmail != listed {
		t.Errorf("offers_email is %v on the list and %v on the single read", listed, one.OffersEmail)
	}
	to = filledTo(t, ctx, h, row)
	if listed != (to != "") {
		t.Errorf("offers_email = %v while reply-recipient fills To with %q", listed, to)
	}
	return listed, to
}

// replyHandlers is the activities surface with the colleague reader compose wires.
func (s *standingEnv) replyHandlers() activities.Handlers {
	return activities.NewHandlers(s.DB()).WithColleagues(compose.ColleagueDomainsFor(s.Pool))
}

// filledTo is the To reply-recipient fills. A withheld row answers 404: the
// composer cannot open on it, so To is empty.
func filledTo(t *testing.T, ctx context.Context, h activities.Handlers, row ids.UUID) string {
	t.Helper()
	code, body := serve(t, ctx, func(w http.ResponseWriter, r *http.Request) { h.GetReplyRecipient(w, r, crmcontracts.Id(row)) })
	switch code {
	case http.StatusOK:
		return readBody[crmcontracts.ReplyRecipient](t, body).Address
	case http.StatusNotFound:
		return ""
	}
	t.Fatalf("reply-recipient answered %d", code)
	return ""
}

func (s *standingEnv) onContactPage(t *testing.T, contact, row ids.UUID) bool {
	t.Helper()
	h := contact360.NewHandlers(contactRoomService(s.Env))
	code, body := serve(t, s.reader(), func(w http.ResponseWriter, r *http.Request) {
		h.GetContact360(w, r, crmcontracts.Id(contact), crmcontracts.GetContact360Params{})
	})
	if code != http.StatusOK {
		t.Fatalf("GET /contacts/{id}/360 answered %d", code)
	}
	page := readBody[crmcontracts.Contact360](t, body)
	rows := []crmcontracts.Activity{}
	if page.Activities != nil {
		rows = append(rows, page.Activities.Data...)
	}
	if page.NextSteps != nil {
		rows = append(rows, page.NextSteps.Data...)
	}
	for _, a := range rows {
		if ids.UUID(a.Id) == row {
			return a.OffersEmail
		}
	}
	t.Fatal("the row is missing from the contact page")
	return false
}

func TestOffersEmailAndTheFilledToAreOneAnswer(t *testing.T) {
	s := setupStanding(t)
	s.WsExec(t, `INSERT INTO workspace_email_domain (domain, source, verified) VALUES ($1, 'admin', true)`, ownDomain)

	t.Run("one readable contact with an ordinary address", func(t *testing.T) {
		link := s.contactAt(t, "Anna Weber", "anna@buyer.test")
		row := s.writtenAbout(t, "task", link)
		if offers, to := s.offeredOnEveryRead(t, row); !offers || to != "anna@buyer.test" {
			t.Errorf("offers_email = %v, To = %q; want true and anna@buyer.test", offers, to)
		}
		if !s.onContactPage(t, link.EntityID, row) {
			t.Error("the contact page says the row offers no email")
		}
	})
	refused := []struct {
		name string
		row  func(t *testing.T) ids.UUID
	}{
		{"an archived contact", func(t *testing.T) ids.UUID {
			link := s.contactAt(t, "Ben Kraus", "ben@buyer.test")
			row := s.writtenAbout(t, "note", link)
			s.WsExec(t, `UPDATE contact SET archived_at = now() WHERE id = $1`, link.EntityID)
			return row
		}},
		{"a shared mailbox", func(t *testing.T) ids.UUID {
			return s.writtenAbout(t, "note", s.contactAt(t, "Buyer Sales", "info@buyer.test"))
		}},
		{"a seat's login", func(t *testing.T) ids.UUID {
			return s.writtenAbout(t, "task", s.contactAt(t, "Rep Two", colleagueAddress))
		}},
		{"an address at our own domain", func(t *testing.T) ids.UUID {
			return s.writtenAbout(t, "call", s.contactAt(t, "Ines Berg", "ines@"+ownDomain))
		}},
		{"a row whose content is withheld", func(t *testing.T) ids.UUID {
			row := s.writtenAbout(t, "task", s.contactAt(t, "Cleo Hahn", "cleo@buyer.test"))
			s.WsExec(t, `UPDATE activity SET audience = 'participants', captured_by = 'human:' || $2::text WHERE id = $1`, row, s.Rep3)
			return row
		}},
	}
	for _, tc := range refused {
		t.Run(tc.name, func(t *testing.T) {
			if offers, to := s.offeredOnEveryRead(t, tc.row(t)); offers || to != "" {
				t.Errorf("offers_email = %v, To = %q; want false and empty", offers, to)
			}
		})
	}
}

// Our domain registered in punycode, a colleague's address spelled in Unicode.
// A written-about row and an email row fold it by the one own-domain rule.
func TestAUnicodeAddressAtOurPunycodeDomainIsAColleagueOnEveryRow(t *testing.T) {
	s := setupStanding(t)
	s.WsExec(t, `INSERT INTO workspace_email_domain (domain, source, verified) VALUES ($1, 'admin', true)`,
		"xn--bcher-kva.example")
	const colleague = "anna@bücher.example"

	t.Run("a written-about row", func(t *testing.T) {
		row := s.writtenAbout(t, "call", s.contactAt(t, "Anna Brandt", colleague))
		if offers, to := s.offeredOnEveryRead(t, row); offers || to != "" {
			t.Errorf("offers_email = %v, To = %q; want false and empty", offers, to)
		}
	})
	t.Run("an email row", func(t *testing.T) {
		ctx, h := s.replier(false), s.replyHandlers()
		seat := threadParty{role: "to", address: colleagueAddress, seat: &s.Rep2}
		client := s.message(t, "punycode-client", "workspace", threadParty{role: "from", address: clientAddress}, seat)
		if to := filledTo(t, ctx, h, client); to != clientAddress {
			t.Fatalf("a client's mail fills To with %q, want %s", to, clientAddress)
		}
		row := s.message(t, "punycode-colleague", "workspace", threadParty{role: "from", address: colleague}, seat)
		if to := filledTo(t, ctx, h, row); to != "" {
			t.Errorf("a colleague's mail fills To with %q, want empty", to)
		}
	})
}

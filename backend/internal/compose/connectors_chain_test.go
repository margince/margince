// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

var (
	chainWorkspace = ids.MustParse("11111111-1111-1111-1111-111111111111")
	chainUser      = ids.MustParse("22222222-2222-2222-2222-222222222222")
)

// connectWith drives the real ConnectConnector with a JSON body.
func connectWith(t *testing.T, h connectorHandlers, provider, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/connectors/"+provider+"/connect", strings.NewReader(body)).WithContext(humanCtx())
	req.Header.Set("Content-Type", "application/json")
	h.ConnectConnector(rec, req, crmcontracts.CaptureProvider(provider))
	return rec
}

// consentState returns the verified state an authorize URL carries.
func consentState(t *testing.T, h connectorHandlers, authorizeURL string) (connectState, *url.URL) {
	t.Helper()
	u, err := url.Parse(authorizeURL)
	if err != nil {
		t.Fatalf("authorize url %q: %v", authorizeURL, err)
	}
	// The handler dated the expiry from its own clock; expiry is
	// connectorstate_test's subject, so this reads the binding at no instant.
	st, err := h.signer.verify(u.Query().Get("state"), time.Time{})
	if err != nil {
		t.Fatalf("authorize url state does not verify: %v", err)
	}
	return st, u
}

func requireRefusal(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status || !strings.Contains(rec.Body.String(), code) {
		t.Fatalf("got %d %s, want %d carrying %q", rec.Code, rec.Body, status, code)
	}
}

func TestConnectSignsTheChainAndTheImportWindowIntoTheState(t *testing.T) {
	h := wiredHandlers()
	rec := connectWith(t, h, "gmail", `{"return_to":"settings","also_connect":"gcal","import_window":"6m"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body)
	}
	var resp crmcontracts.ConnectConnectorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp.AuthorizeUrl == nil {
		t.Fatalf("decode %s: %v", rec.Body, err)
	}
	st, _ := consentState(t, h, *resp.AuthorizeUrl)
	if st.Provider != "gmail" || st.Then != "gcal" || st.WindowMonths != 6 || st.ReturnTo != "settings" {
		t.Errorf("state = %+v, want gmail then gcal, 6 months, back to settings", st)
	}
}

func TestConnectAcceptsMeetingsAsTheReturnSurface(t *testing.T) {
	h := wiredHandlers()
	rec := connectWith(t, h, "gcal", `{"return_to":"meetings"}`)
	var resp crmcontracts.ConnectConnectorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp.AuthorizeUrl == nil {
		t.Fatalf("status %d, body %s: %v", rec.Code, rec.Body, err)
	}
	if st, _ := consentState(t, h, *resp.AuthorizeUrl); st.ReturnTo != returnToMeetings {
		t.Errorf("ReturnTo = %q, want meetings", st.ReturnTo)
	}
}

func TestConnectRefusesWhatItCannotChainOrImport(t *testing.T) {
	for _, tc := range []struct {
		name, provider, body string
		status               int
		code                 string
	}{
		{"a calendar from the other vendor", "gmail", `{"also_connect":"graphcal"}`, http.StatusUnprocessableEntity, codeAlsoConnectMismatch},
		{"a calendar after a calendar", "gcal", `{"also_connect":"gcal"}`, http.StatusUnprocessableEntity, codeAlsoConnectMismatch},
		{"a window the contract does not name", "gmail", `{"import_window":"7m"}`, http.StatusUnprocessableEntity, codeWindowInvalid},
		{"a window on a calendar", "gcal", `{"import_window":"6m"}`, http.StatusUnprocessableEntity, "not_a_mailbox"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireRefusal(t, connectWith(t, wiredHandlers(), tc.provider, tc.body), tc.status, tc.code)
		})
	}
}

func TestConnectRefusesAWindowAboveTheInstallationsCeiling(t *testing.T) {
	h := wiredHandlers()
	h.registry = capture.NewRegistry(nil, nil, liveAuthority{}, nil).WithMaxBackfillMonths(12)

	rec := connectWith(t, h, "gmail", `{"import_window":"24m"}`)

	requireRefusal(t, rec, http.StatusUnprocessableEntity, codeWindowInvalid)
	if !strings.Contains(rec.Body.String(), "3m, 6m, 12m.") {
		t.Errorf("the refusal should name this installation's windows: %s", rec.Body)
	}
}

// Asking for the calendar is asking for a connect that would answer on its own.
// It is refused as connecting the calendar directly would be.
func TestConnectRefusesAChainedCalendarThatCannotConnect(t *testing.T) {
	h := wiredHandlers()
	h.gcalOAuth = nil

	requireRefusal(t, connectWith(t, h, "gmail", `{"also_connect":"gcal"}`), http.StatusNotImplemented, "gcal")
}

func TestImapConnectRefusesAChainAndAnImport(t *testing.T) {
	for _, tc := range []struct{ name, body, code string }{
		{"a calendar", `{"also_connect":"gcal","imap":{"host":"h","username":"u","secret":"s"}}`, codeAlsoConnectMismatch},
		{"an import", `{"import_window":"6m","imap":{"host":"h","username":"u","secret":"s"}}`, codeConnectorUnsupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requireRefusal(t, connectWith(t, wiredHandlers(), "imap", tc.body), http.StatusUnprocessableEntity, tc.code)
		})
	}
}

func TestChainAfterConnectAsksForTheCalendarInsteadOfLanding(t *testing.T) {
	h := wiredHandlers()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/connectors/gmail/callback", nil)

	h.chainAfterConnect(context.Background(), rec, req, connectState{
		Workspace: chainWorkspace, User: chainUser, Provider: "gmail", ReturnTo: returnToSettings, Then: "gcal",
	})

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want 302", rec.Code)
	}
	st, u := consentState(t, h, rec.Header().Get("Location"))
	if got := u.Query().Get("redirect_uri"); got != "https://api.test/v1/connectors/gcal/callback" {
		t.Errorf("redirect_uri = %q, want the gcal callback", got)
	}
	if st.Provider != "gcal" || st.Then != "" || st.ReturnTo != returnToSettings || st.User != chainUser || st.Workspace != chainWorkspace {
		t.Errorf("calendar state = %+v, want gcal for the same human, back to settings, ending the chain", st)
	}
	var nonce string
	for _, c := range rec.Result().Cookies() {
		if c.Name == csrfCookieName("gcal") {
			nonce = c.Value
		}
	}
	if nonce == "" || nonce != st.Nonce {
		t.Errorf("gcal CSRF cookie = %q, want the fresh nonce %q the calendar state carries", nonce, st.Nonce)
	}
}

func TestChainAfterConnectWithNothingToChainLandsAsBefore(t *testing.T) {
	h := wiredHandlers()
	rec := httptest.NewRecorder()

	h.chainAfterConnect(context.Background(), rec, httptest.NewRequest(http.MethodGet, "/", nil),
		connectState{Workspace: chainWorkspace, User: chainUser, Provider: "gmail", ReturnTo: returnToSettings})

	if got, want := rec.Header().Get("Location"), "https://app.test/#/settings/connections/ok/gmail"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}

// A calendar the deployment can no longer serve lands as the calendar's own
// failure: the mailbox is connected, and the screen offers the calendar again.
func TestChainAfterConnectLandsTheCalendarsFailureWhenItsAppIsGone(t *testing.T) {
	h := wiredHandlers()
	h.gcalOAuth = nil
	rec := httptest.NewRecorder()

	h.chainAfterConnect(context.Background(), rec, httptest.NewRequest(http.MethodGet, "/", nil),
		connectState{Workspace: chainWorkspace, User: chainUser, Provider: "gmail", ReturnTo: returnToSettings, Then: "gcal"})

	if got, want := rec.Header().Get("Location"), "https://app.test/#/settings/connections/error/gcal"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}

// recordedImport stands in for the two boundaries the import crosses: the
// provider's count and the backfill run's write.
type recordedImport struct {
	estimate    connector.BackfillEstimate
	estimateErr error
	beginErr    error
	begun       []connector.BackfillEstimate
}

func (f *recordedImport) engine() connectImport {
	return connectImport{
		estimate: func(_ context.Context, provider string, _ ids.UserID, months int) (capture.BackfillPreview, error) {
			if provider != "gmail" || months != 6 {
				return capture.BackfillPreview{}, errors.New("estimated the wrong window")
			}
			return capture.BackfillPreview{BackfillEstimate: f.estimate}, f.estimateErr
		},
		begin: func(_ context.Context, ws ids.UUID, _ string, userID ids.UserID, _ int, estimate connector.BackfillEstimate, startOver bool) (capture.BackfillRun, error) {
			if ws != chainWorkspace || userID.UUID != chainUser || startOver {
				return capture.BackfillRun{}, errors.New("began for the wrong mailbox, or over")
			}
			f.begun = append(f.begun, estimate)
			return capture.BackfillRun{}, f.beginErr
		},
	}
}

func TestTheImportAskedForAtConnectStartsOnlyUnderTheCap(t *testing.T) {
	const held = "?import=6m"
	for _, tc := range []struct {
		name    string
		engine  recordedImport
		started bool
		query   string
	}{
		{"a small mailbox starts", recordedImport{estimate: connector.BackfillEstimate{Messages: 120}}, true, ""},
		{"exactly the cap starts", recordedImport{estimate: connector.BackfillEstimate{Messages: autoImportCap}}, true, ""},
		{"one over the cap waits", recordedImport{estimate: connector.BackfillEstimate{Messages: autoImportCap + 1}}, false, held},
		{"a floor is not known to be under", recordedImport{estimate: connector.BackfillEstimate{Messages: 300, Floor: true}}, false, held},
		{"an uncounted window waits", recordedImport{estimateErr: context.DeadlineExceeded}, false, held},
		{"a run already going lands plain", recordedImport{estimate: connector.BackfillEstimate{Messages: 120}, beginErr: capture.ErrBackfillRunning}, true, ""},
		// The refused enqueue rolled the run back, so nothing else offers it.
		{"a refused enqueue offers the import", recordedImport{estimate: connector.BackfillEstimate{Messages: 120}, beginErr: errBackfillEnqueue}, true, held},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := wiredHandlers()
			h.importer = tc.engine.engine()
			rec := httptest.NewRecorder()

			h.chainAfterConnect(context.Background(), rec, httptest.NewRequest(http.MethodGet, "/", nil), connectState{
				Workspace: chainWorkspace, User: chainUser, Provider: "gmail", ReturnTo: returnToSettings, WindowMonths: 6,
			})

			if got, want := rec.Header().Get("Location"), "https://app.test/#/settings/connections/ok/gmail"+tc.query; got != want {
				t.Errorf("Location = %q, want %q", got, want)
			}
			if began := len(tc.engine.begun) == 1; began != tc.started {
				t.Fatalf("began %d times, want started=%v", len(tc.engine.begun), tc.started)
			}
			if tc.started && tc.engine.begun[0] != tc.engine.estimate {
				t.Errorf("began with estimate %+v, want the one the callback took %+v", tc.engine.begun[0], tc.engine.estimate)
			}
		})
	}
}

// A role with no backfill engine starts nothing and offers nothing it could
// not start either.
func TestTheImportWithNoEngineLandsPlain(t *testing.T) {
	h := wiredHandlers()
	rec := httptest.NewRecorder()

	h.chainAfterConnect(context.Background(), rec, httptest.NewRequest(http.MethodGet, "/", nil), connectState{
		Workspace: chainWorkspace, User: chainUser, Provider: "gmail", ReturnTo: returnToSettings, WindowMonths: 6,
	})

	if got, want := rec.Header().Get("Location"), "https://app.test/#/settings/connections/ok/gmail"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}

// The held import outlives the calendar consent: whatever the calendar leg
// answers, its landing still offers the mail leg's window.
func TestAHeldImportRidesTheChainToTheCalendarsLanding(t *testing.T) {
	h := wiredHandlers()
	over := recordedImport{estimate: connector.BackfillEstimate{Messages: autoImportCap * 3}}
	h.importer = over.engine()
	rec := httptest.NewRecorder()
	h.chainAfterConnect(context.Background(), rec, httptest.NewRequest(http.MethodGet, "/", nil), connectState{
		Workspace: chainWorkspace, User: chainUser, Provider: "gmail", ReturnTo: returnToSettings, Then: "gcal", WindowMonths: 6,
	})
	calendar, u := consentState(t, h, rec.Header().Get("Location"))
	if calendar.WindowMonths != 6 {
		t.Fatalf("calendar state window = %d, want the held 6", calendar.WindowMonths)
	}

	denied := "access_denied"
	back := httptest.NewRecorder()
	h.ConnectorOAuthCallback(back, httptest.NewRequest(http.MethodGet, "/v1/connectors/gcal/callback", nil), "gcal",
		crmcontracts.ConnectorOAuthCallbackParams{Error: &denied, State: u.Query().Get("state")})

	if got, want := back.Header().Get("Location"), "https://app.test/#/settings/connections/denied/gcal?import=6m"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}

// A mail leg's window is a request its own callback has not acted on, so a
// declined mail consent has nothing to offer.
func TestADeclinedMailConsentOffersNoImport(t *testing.T) {
	h := wiredHandlers()
	state := h.signer.sign(connectState{
		Workspace: chainWorkspace, User: chainUser, Provider: "gmail", Nonce: "n",
		ReturnTo: returnToMeetings, WindowMonths: 6,
	}, clocktest.Now(t).Add(time.Minute))
	denied := "access_denied"
	rec := httptest.NewRecorder()

	h.ConnectorOAuthCallback(rec, httptest.NewRequest(http.MethodGet, "/v1/connectors/gmail/callback", nil), "gmail",
		crmcontracts.ConnectorOAuthCallbackParams{Error: &denied, State: state})

	if got, want := rec.Header().Get("Location"), "https://app.test/#/settings/meetings/denied/gmail"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}

// Onboarding's chain stops where the mail leg fails: the calendar is asked for
// only after a granted mailbox, never in place of a declined one.
func TestADeclinedMailConsentNeverAsksForItsCalendar(t *testing.T) {
	h := wiredHandlers()
	state := h.signer.sign(connectState{
		Workspace: chainWorkspace, User: chainUser, Provider: "gmail", Nonce: "n",
		ReturnTo: returnToOnboarding, Then: "gcal",
	}, clocktest.Now(t).Add(time.Minute))
	denied := "access_denied"
	rec := httptest.NewRecorder()

	h.ConnectorOAuthCallback(rec, httptest.NewRequest(http.MethodGet, "/v1/connectors/gmail/callback", nil), "gmail",
		crmcontracts.ConnectorOAuthCallbackParams{Error: &denied, State: state})

	if got, want := rec.Header().Get("Location"), "https://app.test/#/onboarding/connect/denied/gmail"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}

// Onboarding's "Connect calendar" on a connected mailbox asks for the calendar
// alone, which is an ordinary connect with nothing chained after it.
func TestConnectTakesACalendarAloneForOnboarding(t *testing.T) {
	h := wiredHandlers()
	rec := connectWith(t, h, "gcal", `{"return_to":"onboarding"}`)
	var resp crmcontracts.ConnectConnectorResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil || resp.AuthorizeUrl == nil {
		t.Fatalf("status %d, body %s: %v", rec.Code, rec.Body, err)
	}
	st, _ := consentState(t, h, *resp.AuthorizeUrl)
	if st.Provider != "gcal" || st.Then != "" || st.ReturnTo != returnToOnboarding {
		t.Errorf("state = %+v, want gcal alone, back to onboarding", st)
	}
}

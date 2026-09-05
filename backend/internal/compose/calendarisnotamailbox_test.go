// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A calendar connection is not a mailbox, and the mail-shaped ops say so.
//
// Reported from a live installation: a member's Google Calendar row was drawn
// as a mailbox — envelope icon, their own email address beside it — with a
// mail-history import card underneath. The card answered "this mailbox type
// can't be backfilled", which came from the registry finding the gcal connector
// is not a Backfiller. True, and the wrong sentence: a calendar is not a
// mailbox that cannot be enumerated, it is not a mailbox. The member read the
// screen as the product refusing to import their mail.
//
// No database: every refusal here lands before the registry is asked anything,
// which is also what makes them cheap enough to run on every op.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
)

// refusalCode reads the problem body's code, so a test asserting a refusal is
// asserting WHICH one rather than that some 422 happened.
func refusalCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var problem struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decoding the problem body %q: %v", rec.Body.String(), err)
	}
	return problem.Code
}

func TestEveryBackfillOpRefusesACalendarConnection(t *testing.T) {
	h := backfillHandlers{registry: capture.NewRegistry(nil, nil, liveAuthority{}, nil)}
	for _, calendar := range []crmcontracts.CaptureProvider{
		crmcontracts.CaptureProviderGcal, crmcontracts.CaptureProviderGraphcal,
	} {
		ops := map[string]func(http.ResponseWriter, *http.Request){
			"preview": func(w http.ResponseWriter, r *http.Request) {
				h.PreviewConnectorBackfill(w, r, calendar)
			},
			"start": func(w http.ResponseWriter, r *http.Request) {
				h.StartConnectorBackfill(w, r, calendar)
			},
			"status": func(w http.ResponseWriter, r *http.Request) {
				h.GetConnectorBackfillStatus(w, r, calendar)
			},
			"cancel": func(w http.ResponseWriter, r *http.Request) {
				h.CancelConnectorBackfill(w, r, calendar)
			},
		}
		for op, invoke := range ops {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost,
				"/v1/connectors/"+string(calendar)+"/backfill/"+op,
				strings.NewReader(`{"window":"3m"}`)).WithContext(humanCtx())
			req.Header.Set("Content-Type", "application/json")

			invoke(rec, req)

			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("%s %s = %d, want 422", calendar, op, rec.Code)
				continue
			}
			if got := refusalCode(t, rec); got != "not_a_mailbox" {
				t.Errorf("%s %s refused with %q, want not_a_mailbox — connector_unsupported says the "+
					"provider cannot enumerate a mailbox, and a calendar has none to enumerate",
					calendar, op, got)
			}
		}
	}
}

// The control: a mailbox passes this door and meets its own refusals further
// in. Without it, a guard that refused everything would satisfy every assertion
// above. The body is deliberately unreadable, so the preview stops at the
// window it was not given — one door past the kind check and still short of the
// registry, which needs a database this test does not have.
func TestABackfillOpOnAMailboxPassesTheKindCheck(t *testing.T) {
	h := backfillHandlers{registry: capture.NewRegistry(nil, nil, liveAuthority{}, nil)}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/connectors/imap/backfill/preview",
		strings.NewReader("not json")).WithContext(humanCtx())
	req.Header.Set("Content-Type", "application/json")

	h.PreviewConnectorBackfill(rec, req, crmcontracts.CaptureProviderImap)

	if got := refusalCode(t, rec); got != "window_required" {
		t.Errorf("imap preview refused with %q, want window_required — imap is the self-hostable "+
			"mailbox engine and must reach its own doors", got)
	}
}

func TestSettingAMailPostureRefusesACalendarConnection(t *testing.T) {
	h := wiredHandlers()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/v1/connectors/gcal/mail-posture",
		strings.NewReader(`{"posture":"classified"}`)).WithContext(humanCtx())
	req.Header.Set("Content-Type", "application/json")

	h.SetConnectorMailPosture(rec, req, crmcontracts.CaptureProviderGcal)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422 — a calendar brings in no mail to take a posture towards", rec.Code)
	}
	if got := refusalCode(t, rec); got != "not_a_mailbox" {
		t.Errorf("refused with %q, want not_a_mailbox", got)
	}
}

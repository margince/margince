// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// One connect, up to two consents and an import. A mail consent may ask for its
// paired calendar straight after (also_connect) and for a past-email window
// (import_window). Both ride the signed state, so the callback decides what
// follows and nothing in the address bar can replay or widen it.

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// autoImportCap is the largest estimate an import asked for at connect starts
// on by itself. Above it the reader sees the count and starts it with one
// press; the landing says which happened, so no screen repeats this number.
const autoImportCap = 5000

// autoImportEstimateTimeout bounds the count the callback takes before it
// redirects. A window too big to count in time lands held, which is the cap's
// answer for it anyway.
const autoImportEstimateTimeout = 5 * time.Second

const codeAlsoConnectMismatch = "also_connect_mismatch"

var errImportUnwired = errors.New("connector callback: no backfill engine is wired, so the import asked for at connect cannot start")

// connectImport is the backfill engine the callback starts an import on: the
// registry's estimate, and backfillHandlers.begin, the run the start endpoint
// opens. Compose wires both; a role without the backfill surface leaves them nil.
type connectImport struct {
	estimate func(ctx context.Context, provider string, userID ids.UserID, months int) (capture.BackfillPreview, error)
	begin    func(ctx context.Context, ws ids.UUID, provider string, userID ids.UserID, months int, estimate connector.BackfillEstimate, startOver bool) (capture.BackfillRun, error)
}

// startsOnItsOwn is the auto-import rule. A floor is not known to be under the
// cap, so it waits for the reader like any count above it.
func startsOnItsOwn(estimate connector.BackfillEstimate) bool {
	return !estimate.Floor && estimate.Messages <= autoImportCap
}

// start opens the leg's window when its count is under the cap. False with no
// error means the import is held for the reader: over the cap, or uncounted,
// since an uncounted window is never started blind.
func (c connectImport) start(ctx context.Context, st connectState) (bool, error) {
	if c.estimate == nil || c.begin == nil {
		return false, errImportUnwired
	}
	userID := ids.From[ids.UserKind](st.User)
	sizeCtx, cancel := context.WithTimeout(ctx, autoImportEstimateTimeout)
	preview, err := c.estimate(sizeCtx, st.Provider, userID, st.WindowMonths)
	cancel()
	if err != nil {
		slog.WarnContext(ctx, "connector callback: the import window could not be counted, so it waits for the reader",
			"err", err, "provider", st.Provider)
		return false, nil
	}
	if !startsOnItsOwn(preview.BackfillEstimate) {
		return false, nil
	}
	_, err = c.begin(ctx, st.Workspace, st.Provider, userID, st.WindowMonths, preview.BackfillEstimate, false)
	return err == nil, err
}

// heldOnLeg is the window a landing offers before this leg does anything. Only a
// calendar leg carries one, forwarded by the mail leg that held it.
// A mail leg's window is a request its own callback has not acted on yet.
func heldOnLeg(st connectState) int {
	if IsMailProvider(st.Provider) {
		return 0
	}
	return st.WindowMonths
}

// chainAfterConnect runs what the consent asked for once its grant is stored:
// the import, then the paired calendar's consent instead of the landing.
func (h connectorHandlers) chainAfterConnect(ctx context.Context, w http.ResponseWriter, r *http.Request, st connectState) {
	held := heldOnLeg(st)
	if IsMailProvider(st.Provider) && st.WindowMonths > 0 {
		started, err := h.importer.start(ctx, st)
		switch {
		case errors.Is(err, capture.ErrBackfillRunning):
			// A reconnect during a live import: the running one already shows.
			slog.InfoContext(ctx, "connector callback: an import is already running for this mailbox",
				"provider", st.Provider)
		case err != nil:
			// The mailbox is connected either way. A refused start persisted no run,
			// so the landing offers it again; an unwired engine has nothing to offer.
			slog.ErrorContext(ctx, "connector callback: starting the import asked for at connect",
				"err", err, "provider", st.Provider)
			if !errors.Is(err, errImportUnwired) {
				held = st.WindowMonths
			}
		case !started:
			held = st.WindowMonths
		}
	}
	if st.Then == "" {
		http.Redirect(w, r, h.landingURL(outcomeOK, st.ReturnTo, st.Provider, held), http.StatusFound)
		return
	}
	// Resolved again rather than trusted from the connect: a stored app is read
	// per request, and minutes of consent screens have passed since.
	app, ok, err := h.oauthApp(ctx, st.Then)
	if err != nil || !ok {
		slog.ErrorContext(ctx, "connector callback: the chained calendar has no usable OAuth app",
			"err", err, "provider", st.Then)
		http.Redirect(w, r, h.landingURL(outcomeError, st.ReturnTo, st.Then, held), http.StatusFound)
		return
	}
	next := connectState{Workspace: st.Workspace, User: st.User, Provider: st.Then, ReturnTo: st.ReturnTo, WindowMonths: held}
	http.Redirect(w, r, h.mintConsent(w, app, next), http.StatusFound)
}

// mintConsent opens one provider's consent. CSRF: a random nonce goes into both
// a SameSite=Lax cookie and the signed state. The callback requires them to
// match, so a victim can't complete an attacker-initiated flow.
func (h connectorHandlers) mintConsent(w http.ResponseWriter, app oauthApp, st connectState) string {
	st.Nonce = setCSRFNonce(w, csrfCookieName(st.Provider), connectorCookiePath)
	st.Version = stateVersionNamespacedCSRF
	state := h.signer.sign(st, time.Now().Add(connectStateTTL))
	return app.authCodeURL(state, h.callbackURL(st.Provider))
}

// connectAsk is what an OAuth connect asks of its callback beyond its own grant.
type connectAsk struct {
	returnTo     string
	then         string
	windowMonths int
}

// decodeConnectRequest reads the optional OAuth connect body. False means the
// refusal is already written.
func (h connectorHandlers) decodeConnectRequest(w http.ResponseWriter, r *http.Request, provider string) (connectAsk, bool) {
	ask := connectAsk{returnTo: returnToOnboarding}
	// ContentLength 0 is the reliable "no body" signal. -1 (chunked) must still
	// be decoded, or a chunked request's fields are dropped and a malformed
	// chunked body skips rejection entirely.
	if r.ContentLength == 0 {
		return ask, true
	}
	var req crmcontracts.ConnectConnectorRequest
	if !httperr.Decode(w, r, &req) {
		return ask, false
	}
	if req.ReturnTo != nil {
		ask.returnTo = landingSurface(string(*req.ReturnTo))
	}
	ok := true
	if req.ImportWindow != nil {
		ask.windowMonths, ok = h.connectWindow(w, r, provider, string(*req.ImportWindow))
	}
	if ok && req.AlsoConnect != nil {
		ask.then, ok = h.chainedCalendar(w, r, provider, string(*req.AlsoConnect))
	}
	return ask, ok
}

// landingSurface keeps return_to a closed set: a value it does not name lands
// on onboarding, as an absent one does.
func landingSurface(asked string) string {
	switch asked {
	case returnToSettings, returnToMeetings:
		return asked
	default:
		return returnToOnboarding
	}
}

// connectWindow admits import_window only on a mailbox, and only as a window
// this installation offers, the set the backfill start admits.
func (h connectorHandlers) connectWindow(w http.ResponseWriter, r *http.Request, provider, asked string) (int, bool) {
	if !mailboxOnly(w, r, crmcontracts.CaptureProvider(provider), noMailHistory) {
		return 0, false
	}
	offered := h.registry.OfferedBackfillWindows()
	months, ok := windowMonths(asked)
	if ok && slices.Contains(offered, months) {
		return months, true
	}
	names := make([]string, 0, len(offered))
	for _, m := range offered {
		names = append(names, monthsWindow(m))
	}
	httperr.Write(w, r, &httperr.DetailedError{
		Status: http.StatusUnprocessableEntity, Code: codeWindowInvalid,
		Detail: "The import window must be one of " + strings.Join(names, ", ") + ".",
	})
	return 0, false
}

// chainedCalendar admits also_connect only as this mailbox's own calendar, and
// only while that calendar could be connected directly.
func (h connectorHandlers) chainedCalendar(w http.ResponseWriter, r *http.Request, provider, asked string) (string, bool) {
	if pairedCalendar[provider] != asked {
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusUnprocessableEntity, Code: codeAlsoConnectMismatch,
			Detail: "also_connect names the calendar on the same account: " + calendarPairings() + ".",
		})
		return "", false
	}
	avail := h.connectability(r.Context(), asked)
	if avail.err != nil {
		writeConnectorAppFailure(w, r, avail.err)
		return "", false
	}
	if avail.reason != connectReady {
		writeConnectorUnavailable(w, r, asked, h.registry != nil)
		return "", false
	}
	return asked, true
}

func calendarPairings() string {
	pairs := make([]string, 0, len(pairedCalendar))
	for mail, calendar := range pairedCalendar {
		pairs = append(pairs, calendar+" after "+mail)
	}
	slices.Sort(pairs)
	return strings.Join(pairs, ", ")
}

// imapAsksNothingMore refuses what only an OAuth connect can carry: an IMAP
// mailbox has no paired calendar and cannot read backward from a date.
func imapAsksNothingMore(w http.ResponseWriter, r *http.Request, req crmcontracts.ConnectConnectorRequest) bool {
	switch {
	case req.AlsoConnect != nil:
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusUnprocessableEntity, Code: codeAlsoConnectMismatch,
			Detail: "An IMAP mailbox has no calendar to connect with it.",
		})
	case req.ImportWindow != nil:
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusUnprocessableEntity, Code: codeConnectorUnsupported,
			Detail: "An IMAP mailbox cannot import past email. New mail still comes in.",
		})
	default:
		return true
	}
	return false
}

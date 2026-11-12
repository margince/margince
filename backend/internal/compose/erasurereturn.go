// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The one door that undoes an erasure. An erased subject asks on a booking page
// to be stored again. They are stored only when they answer a link mailed to
// the address. The ask answers every caller the same way, before any lookup, so
// its answer and timing say nothing about the address. The confirm lifts the
// erasure, stores the contact and records the consent in one transaction.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/returnlink"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/platform/mailer"
	"github.com/margince/margince/backend/internal/platform/ratelimit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// returnSource names the door on the contact and the consent it records.
const returnSource = "erasure_return"

// returnSendTimeout bounds the lookup and the mail that run after the answer.
const returnSendTimeout = 30 * time.Second

// erasureReturnHandlers serves requestBookingReturn and confirmBookingReturn.
// The booking edge in front of them has already resolved the slug, throttled
// the caller and bound a system principal.
type erasureReturnHandlers struct {
	pool           *pgxpool.Pool
	contacts       *contacts.Store
	consent        *consent.Store
	returnMail     mailer.Mailer
	returnLinkBase string
	perAddress     *ratelimit.Limiter
	now            func() time.Time
	// sendDone is a test seam: called when the work after the answer ends.
	sendDone func()
}

func newErasureReturnHandlers(pool *pgxpool.Pool) erasureReturnHandlers {
	return erasureReturnHandlers{
		pool:     pool,
		contacts: newContactsStore(InstallationDB(pool)),
		consent:  consent.NewStore(InstallationDB(pool)),
		// One mailbox gets a few links an hour, however many callers ask.
		perAddress: ratelimit.New("erasure-return/per-address", ratelimit.FailClosed, 3, time.Hour),
		now:        time.Now,
	}
}

// returnLinkSignerNow is the signer under the installation's current root key.
func returnLinkSignerNow() returnlink.Signer {
	return returnlink.NewSigner(keyvault.DerivedKey(config.Bound(), returnlink.KeyLabel))
}

// RequestBookingReturn implements (POST /public/booking/{host_slug}/return).
func (h erasureReturnHandlers) RequestBookingReturn(w http.ResponseWriter, r *http.Request, hostSlug string) {
	var body crmcontracts.RequestBookingReturnJSONRequestBody
	if !httperr.Decode(w, r, &body) {
		return
	}
	email, err := values.ParseEmail(string(body.Email))
	if err != nil {
		httperr.Write(w, r, httperr.Validation("email", "invalid_email", "a valid email address is required"))
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		httperr.Write(w, r, httperr.Validation("name", "required", "the name is required"))
		return
	}
	if body.Consent.PolicyVersion == "" || body.Consent.Wording == nil || strings.TrimSpace(*body.Consent.Wording) == "" {
		httperr.Write(w, r, httperr.Validation("consent", "required", "the consent wording and its version are required"))
		return
	}
	var requested *ids.UUID
	if body.Consent.PurposeId != nil {
		named := ids.UUID(*body.Consent.PurposeId)
		requested = &named
	}
	purpose, err := bookingConsentAdapter{store: h.consent}.ScopedPurpose(r.Context(), requested)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	if !h.perAddress.Allow(strings.ToLower(email.String())) {
		httperr.Write(w, r, apperrors.ErrBudgetExceeded)
		return
	}
	request := returnlink.Request{
		Slug: hostSlug, Name: strings.TrimSpace(body.Name), Email: email.String(), PurposeID: purpose,
		PolicyVersion: body.Consent.PolicyVersion, Wording: *body.Consent.Wording,
	}
	// Everything that depends on the address runs after the answer, so the
	// answer and its timing are the same for every address. A failure there is
	// logged for the operator and never changes what the caller saw.
	workCtx, release := context.WithTimeout(context.WithoutCancel(r.Context()), returnSendTimeout)
	go func() {
		defer release()
		if h.sendDone != nil {
			defer h.sendDone()
		}
		defer httpserver.RecoverDetached("erasure return link")
		if err := h.mailReturnLink(workCtx, request); err != nil {
			slog.ErrorContext(workCtx, "erasure return link not mailed", "err", err)
		}
	}()
	w.WriteHeader(http.StatusAccepted)
}

// mailReturnLink mails the link when the address is erased, and does nothing
// otherwise.
func (h erasureReturnHandlers) mailReturnLink(ctx context.Context, request returnlink.Request) error {
	if h.returnMail == nil || h.returnLinkBase == "" {
		return nil
	}
	var erased bool
	if err := InstallationDB(h.pool).Tx(ctx, func(tx pgx.Tx) error {
		var err error
		request.ErasedAt, erased, err = privacy.EmailErasedAt(ctx, tx, request.Email)
		return err
	}); err != nil || !erased {
		return err
	}
	token, err := returnLinkSignerNow().Sign(request, h.now().Add(returnlink.Lifetime))
	if err != nil {
		return err
	}
	link := h.returnLinkBase + "/#/book/" + url.PathEscape(request.Slug) + "?return=" + token
	words := mailcopy.For(string(identity.BaseLanguageForPrompt(ctx, h.pool)))
	message := words.ReturnMessage(link)
	if err := mailer.SendProduct(ctx, h.returnMail, request.Email, message.Title, message); err != nil {
		return fmt.Errorf("sending the erasure return link: %w", err)
	}
	return nil
}

// ConfirmBookingReturn implements (POST /public/booking/{host_slug}/return/confirm).
func (h erasureReturnHandlers) ConfirmBookingReturn(w http.ResponseWriter, r *http.Request, hostSlug string) {
	var body crmcontracts.ConfirmBookingReturnJSONRequestBody
	if !httperr.Decode(w, r, &body) {
		return
	}
	request, err := returnLinkSignerNow().Verify(body.Token, h.now())
	if err != nil || !strings.EqualFold(request.Slug, hostSlug) {
		writeReturnLinkSpent(w, r)
		return
	}
	err = h.storeReturned(r.Context(), request)
	if errors.Is(err, privacy.ErrLiftNotCurrent) {
		writeReturnLinkSpent(w, r)
		return
	}
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeReturnLinkSpent is the one answer for every link that does not work: a
// bad signature, an expired, spent or foreign link all read the same.
func writeReturnLinkSpent(w http.ResponseWriter, r *http.Request) {
	httperr.Write(w, r, httperr.Validation("token", "link_spent",
		"This link was used already or has expired. Ask for a new one on the booking page."))
}

// storeReturned lifts the erasure, stores the contact and records the consent
// in one transaction. The contact is written while the address is still on the
// list, under the booking page's admission. The lift comes last and checks that
// the link still names the current erasure. Its refusal rolls all of it back.
func (h erasureReturnHandlers) storeReturned(ctx context.Context, request returnlink.Request) error {
	return database.WithWorkspaceWriteTx(ctx, h.pool, func(ctx context.Context) error {
		contactID, err := h.contacts.EnsureContactByEmail(ctx, request.Name, request.Email, returnSource, storekit.AdmitSelfBooking)
		if err != nil {
			return err
		}
		granted, err := h.consent.Record(ctx, returnConsentRecord(ids.From[ids.ContactKind](contactID), request))
		if err != nil {
			return err
		}
		return InstallationDB(h.pool).Tx(ctx, func(tx pgx.Tx) error {
			return privacy.LiftEmailSuppressionTx(ctx, tx, request.Email, request.ErasedAt, granted.ProofEventID, contactID)
		})
	})
}

// returnConsentRecord is the grant the subject gave on the booking page. The
// mailbox answered the link, so the grant may replace a withdrawal on record,
// which an anonymous booking may not. It always writes its own proof row, which
// the lift names.
func returnConsentRecord(subject ids.ContactID, request returnlink.Request) consent.RecordInput {
	source := returnSource
	return consent.RecordInput{
		ContactID:        subject,
		PurposeID:        ids.From[ids.PurposeKind](request.PurposeID),
		NewState:         "granted",
		Source:           &source,
		PolicyText:       &request.Wording,
		PolicyVersion:    &request.PolicyVersion,
		ProveReassertion: true,
	}
}

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// The anonymous booking surface (feedback/14 — B-EP09.14 book.html):
// an unguessable slug resolves to (workspace, host) with no session and
// no workspace header, the availability read answers free/busy slots
// only, and the booking POST captures the booker as a person
// (idempotent on email), records the MANDATORY consent passthrough, and
// books the slot — three governed writes in sequence, each audited on
// its own path. A 409 after person+consent stands: the subject DID
// submit the form and the consent (capture semantics, recorded in the
// batch decision file).

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// BookingPage is the slug's resolution: which workspace, whose calendar.
type BookingPage struct {
	HostUserID ids.UserID
}

// ResolveBookingPage answers the slug→host lookup the public middleware runs
// before it may do anything else. The slug names the HOST, and only the host:
// the installation the request lands in is resolved from the installation
// itself, not read off a row an anonymous caller chose. An unknown or revoked
// slug reads as absent.
func (s *Store) ResolveBookingPage(ctx context.Context, slug string) (BookingPage, error) {
	var page BookingPage
	err := database.WithInfraTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx,
			`SELECT host_user_id FROM booking_page WHERE slug = $1 AND revoked_at IS NULL`,
			slug).Scan(&page.HostUserID)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		return err
	})
	if err != nil {
		return BookingPage{}, err
	}
	return page, nil
}

// SeedBookingPageTx mints a host's public page inside the caller's
// transaction (the workspace bootstrap seeds one for the admin). The
// slug is a public identifier, not a credential — unguessable so the
// URL cannot be enumerated, stored plaintext because it IS the URL.
func SeedBookingPageTx(ctx context.Context, tx pgx.Tx, hostUserID ids.UserID) (string, error) {
	var buf [24]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", fmt.Errorf("activities: booking slug entropy: %w", err)
	}
	slug := base64.RawURLEncoding.EncodeToString(buf[:])
	if _, err := tx.Exec(ctx, `
		INSERT INTO booking_page (host_user_id, slug)
		VALUES ($1, $2)`,
		hostUserID, slug); err != nil {
		return "", fmt.Errorf("activities: seed booking page: %w", err)
	}
	return slug, nil
}

// PersonEnsurer is the people seam of the public capture path (compose
// injects it — activities never imports a sibling).
type PersonEnsurer interface {
	EnsurePersonByEmail(ctx context.Context, fullName, email, source string) (ids.UUID, error)
}

// BookingConsent is the CaptureConsent passthrough: the purpose and the
// exact wording/version the anonymous booker was shown.
type BookingConsent struct {
	PurposeID     ids.UUID
	PolicyVersion string
	Wording       *string
	// Marketing is the affirmative tick, absent when the form carried none or
	// the box was left unchecked. It names a question to ASK, never a grant to
	// write — see BookingMarketing.
	Marketing *BookingMarketing
}

// BookingMarketing is an affirmative marketing tick on a booking form.
//
// It is deliberately not a grant. This surface is anonymous: whoever posts it
// proves only that they know an email address, so a tick recorded as consent
// would let a stranger subscribe somebody else. What the tick buys is one
// mailed confirmation link, and the grant exists only if the address's own
// holder spends it.
//
// PolicyVersion and Wording are carried and validated but reach no row today:
// the grant is written by the confirm submission, which supplies the
// confirmation page's own wording and no version at all. The gap and why the
// refusals stay are on bookingConsentAdapter.askMarketing.
type BookingMarketing struct {
	PurposeID     ids.UUID
	PolicyVersion string
	Wording       string
	// TickedFrom is the address the form was submitted with, and it is here so
	// the capturer can refuse to mail the link somewhere else.
	//
	// It is not the address the link goes to. The mint derives that from the
	// person's own record, which is the security property — a caller who could
	// name the destination could name a stranger's. But an email resolves to an
	// EXISTING person whenever one holds it, including on a NON-primary
	// address, and the mint then posts to that person's primary instead. So a
	// tick submitted from somebody's old address reaches their current one,
	// asking them to confirm a subscription requested from an address they did
	// not use. Verified: posting a secondary address delivered the link to the
	// primary.
	//
	// Empty when the door has no submitted address to compare — the
	// authenticated booking names a linked person rather than typing an email,
	// so there is no second address for the two to disagree about.
	TickedFrom string
}

// ConsentCapturer records the booker's consent grant (the consent
// module behind a seam). ValidatePurpose runs BEFORE any write so a
// bogus purpose refuses the whole capture — no person row without a
// recordable consent.
//
// ValidateMarketingPurpose is the same before-any-write probe for the marketing
// tick, and it is a SECOND method rather than an argument to the first because
// the two admit different purposes: the booking lane admits exactly one
// operational purpose, and the marketing question admits only a purpose that
// requires double opt-in. Folding them would give one call site two meanings.
type ConsentCapturer interface {
	ValidatePurpose(ctx context.Context, purposeID ids.UUID) error
	ValidateMarketingPurpose(ctx context.Context, purposeID ids.UUID) error
	CaptureBookingConsent(ctx context.Context, personID ids.UUID, consent BookingConsent) error
}

// WithPublicBooking wires the public capture seams.
func (h Handlers) WithPublicBooking(people PersonEnsurer, consent ConsentCapturer) Handlers {
	h.publicPeople = people
	h.publicConsent = consent
	return h
}

// GetPublicAvailability implements (GET /public/booking/{host_slug}/availability).
// The middleware already resolved the slug and bound workspace + the
// system principal; the handler re-resolves for the host id (the
// lookup is the same global read) and answers slots only.
func (h Handlers) GetPublicAvailability(w http.ResponseWriter, r *http.Request, hostSlug string, params crmcontracts.GetPublicAvailabilityParams) {
	page, err := h.store.ResolveBookingPage(r.Context(), hostSlug)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	duration := defaultSlotDuration
	if params.DurationMinutes != nil {
		duration = time.Duration(*params.DurationMinutes) * time.Minute
	}
	slots, truncated, err := h.store.Availability(r.Context(), page.HostUserID, params.From, params.To, duration)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, map[string]any{"slots": slots, "truncated": truncated})
}

// BookPublicMeeting implements (POST /public/booking/{host_slug}):
// consent-shape check → person (idempotent on email) → consent grant →
// booking. The 201 discloses nothing beyond the slot itself.
func (h Handlers) BookPublicMeeting(w http.ResponseWriter, r *http.Request, hostSlug string, _ crmcontracts.BookPublicMeetingParams) {
	if h.publicPeople == nil || h.publicConsent == nil {
		// Fail closed: a process role composed without the capture seams
		// must refuse, not book without consent.
		httperr.Write(w, r, apperrors.ErrPermissionDenied)
		return
	}
	page, err := h.store.ResolveBookingPage(r.Context(), hostSlug)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}

	var req crmcontracts.BookPublicMeetingJSONRequestBody
	if !httperr.Decode(w, r, &req) {
		return
	}
	if req.Booker.Name == "" || req.Booker.Email == "" {
		httperr.Write(w, r, httperr.Validation("booker", "required", "booker.name and booker.email are required"))
		return
	}
	if !req.End.After(req.Start) {
		httperr.Write(w, r, httperr.Validation("end", "invalid", "end must follow start"))
		return
	}
	// Consent is mandatory and validated BEFORE any write: a public
	// capture surface may not create a person it cannot attach a
	// recordable consent to.
	if req.Consent.PolicyVersion == "" {
		httperr.Write(w, r, httperr.Validation("consent.policy_version", "required", "the consent wording version shown to the booker is required"))
		return
	}
	// Checked HERE, not left to the consent writer, for the reason the comment
	// above gives: EnsurePersonByEmail below commits a person row before the
	// grant is attempted, so a refusal down there would leave one behind — and
	// this door is unauthenticated, so a caller omitting the field in a loop
	// grows the person table one rejected request at a time.
	if req.Consent.Wording == nil || strings.TrimSpace(*req.Consent.Wording) == "" {
		httperr.Write(w, r, httperr.Validation("consent.wording", "required", "the consent wording shown to the booker is required"))
		return
	}
	purposeID := ids.UUID(req.Consent.PurposeId)
	if err := h.publicConsent.ValidatePurpose(r.Context(), purposeID); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	// Probed here for the reason the operational half above is: the ensure
	// below commits a person row, so a refusal after it would leave one behind
	// on an unauthenticated door.
	marketing, ok := h.admitBookingMarketing(w, r, req.Consent, string(req.Booker.Email))
	if !ok {
		return
	}

	personID, err := h.publicPeople.EnsurePersonByEmail(r.Context(), req.Booker.Name, string(req.Booker.Email), "public_booking")
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	if err := h.publicConsent.CaptureBookingConsent(r.Context(), personID, BookingConsent{
		PurposeID:     purposeID,
		PolicyVersion: req.Consent.PolicyVersion,
		Wording:       req.Consent.Wording,
		Marketing:     marketing,
	}); err != nil {
		writeStoreErr(w, r, err)
		return
	}

	subject := "Meeting"
	if req.Subject != nil && *req.Subject != "" {
		subject = *req.Subject
	}
	_, err = h.store.BookMeeting(r.Context(), BookMeetingInput{
		Host:    page.HostUserID,
		Start:   req.Start,
		End:     req.End,
		Subject: subject,
		Links:   []ActivityLinkInput{{EntityType: "person", EntityID: personID}},
		Source:  "public_booking",
	})
	if err != nil {
		var slotTaken *SlotTakenError
		if errors.As(err, &slotTaken) {
			httperr.Write(w, r, httperr.Duplicate("slot_taken", ""))
			return
		}
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, map[string]any{
		"start": req.Start, "end": req.End,
	})
}

// admitBookingMarketing settles the marketing tick BEFORE any write: it refuses
// a malformed one and probes the purpose, so a form carrying a bad tick never
// reaches the person-ensure that would otherwise leave a row behind.
//
// Both booking doors call it, and that is the point. The public form and the
// authenticated one carry the same passthrough schema, so a check living in one
// handler would leave the other admitting a tick the first refuses.
//
// The wording is required and not merely stored: the grant this tick eventually
// produces is evidence, and a grant that cannot say what the subject read when
// they ticked is not demonstrable. The operational half beside it already
// refuses on the same ground.
// It takes the whole CaptureConsent rather than its marketing member because
// that member is an ANONYMOUS struct in the generated contract, and Go has no
// way to name one: a parameter of that type would have to restate the whole
// literal, field tags included.
func (h Handlers) admitBookingMarketing(w http.ResponseWriter, r *http.Request, c crmcontracts.CaptureConsent, tickedFrom string) (*BookingMarketing, bool) {
	m := c.Marketing
	if m == nil {
		return nil, true
	}
	if strings.TrimSpace(m.PolicyVersion) == "" {
		httperr.Write(w, r, httperr.Validation("consent.marketing.policy_version", "required",
			"the marketing wording version shown to the subject is required"))
		return nil, false
	}
	if strings.TrimSpace(m.Wording) == "" {
		httperr.Write(w, r, httperr.Validation("consent.marketing.wording", "required",
			"the marketing wording shown to the subject is required"))
		return nil, false
	}
	purposeID := ids.UUID(m.PurposeId)
	if err := h.publicConsent.ValidateMarketingPurpose(r.Context(), purposeID); err != nil {
		writeStoreErr(w, r, err)
		return nil, false
	}
	return &BookingMarketing{
		PurposeID:     purposeID,
		PolicyVersion: m.PolicyVersion,
		Wording:       m.Wording,
		TickedFrom:    tickedFrom,
	}, true
}

// captureBookingConsent records the authenticated booking's optional
// CaptureConsent passthrough BEFORE the slot commits, on the same seam the
// anonymous page rides. It reports whether the booking may proceed; a false
// return has already written the refusal.
//
// The stance on ordering is deliberate: a slot_taken 409 AFTER the grant leaves
// the grant standing, because the subject did give it. Recording is mandatory
// once the field is present, so a process role composed without the consent
// seam refuses rather than booking with an unrecorded consent.
func (h Handlers) captureBookingConsent(w http.ResponseWriter, r *http.Request, c *crmcontracts.CaptureConsent, links []ActivityLinkInput) bool {
	if c == nil {
		return true
	}
	if h.publicConsent == nil {
		httperr.Write(w, r, apperrors.ErrPermissionDenied)
		return false
	}
	if c.PolicyVersion == "" {
		httperr.Write(w, r, httperr.Validation("consent.policy_version", "required",
			"the consent wording version shown to the subject is required"))
		return false
	}
	// Both halves of the proof row are settled before anything is written: a
	// grant that cannot say what the subject read is not demonstrable.
	if c.Wording == nil || strings.TrimSpace(*c.Wording) == "" {
		httperr.Write(w, r, httperr.Validation("consent.wording", "required",
			"the consent wording shown to the subject is required"))
		return false
	}
	personID, ok := consentSubjectLink(w, r, links)
	if !ok {
		return false
	}
	purposeID := ids.UUID(c.PurposeId)
	if err := h.publicConsent.ValidatePurpose(r.Context(), purposeID); err != nil {
		writeStoreErr(w, r, err)
		return false
	}
	// No submitted address: this door names a linked person instead of typing
	// an email, so there is no second address the link could disagree with.
	marketing, ok := h.admitBookingMarketing(w, r, *c, "")
	if !ok {
		return false
	}
	if err := h.publicConsent.CaptureBookingConsent(r.Context(), personID, BookingConsent{
		PurposeID:     purposeID,
		PolicyVersion: c.PolicyVersion,
		Wording:       c.Wording,
		Marketing:     marketing,
	}); err != nil {
		writeStoreErr(w, r, err)
		return false
	}
	return true
}

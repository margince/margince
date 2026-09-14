// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The no-login preference-center transport (B-E11.32). The public
// middleware has already resolved the token to (workspace, contact) and
// bound the workspace GUC plus the system principal; each handler
// re-resolves the token for the contact id (the same infra read) and then
// drives the consent engine. An unknown or revoked token reads as absent
// (404) — the surface is never a consent-state oracle, and a GET/prefetch
// on the unsubscribe path never withdraws (only POST is routed to it).

import (
	"context"
	"errors"
	"net/http"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// GetPreferenceCenter implements (GET /public/preferences/{token}): the
// recipient's per-purpose consent state, recognized without any login.
func (h Handlers) GetPreferenceCenter(w http.ResponseWriter, r *http.Request, token string) {
	ref, err := h.store.ResolvePreferenceToken(r.Context(), token)
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	view, err := h.store.PublicPreferenceView(r.Context(), ref)
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	writePreferenceCenter(w, view, nil)
}

// OneClickUnsubscribe implements (POST /public/preferences/{token}/unsubscribe):
// the RFC 8058 one-click endpoint. No login, no confirmation page, a fixed
// body. When a purpose is named only that purpose is withdrawn (the one
// the message was sent under); otherwise every withdrawable purpose is.
// Idempotent — re-asserting a withdrawal writes no second proof row.
func (h Handlers) OneClickUnsubscribe(w http.ResponseWriter, r *http.Request, token string, params crmcontracts.OneClickUnsubscribeParams) {
	if err := requireOneClickBody(r); err != nil {
		httperr.Write(w, r, err)
		return
	}
	subject, scoped, viaCredential, err := h.oneClickSubject(r.Context(), token, params)
	if errors.Is(err, errNoConsentSubject) {
		// A lead or a bare address: no per-purpose state exists to withdraw,
		// so the press records a stop instead. See StopForCredentialTx.
		h.stopForCredential(w, r, token)
		return
	}
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	withdrawn, err := h.unsubscribe(r.Context(), subject, scoped, viaCredential)
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	if withdrawn == nil {
		withdrawn = []string{}
	}
	httperr.WriteJSON(w, http.StatusOK, map[string]any{"unsubscribed": answeredKeys(withdrawn, viaCredential)})
}

// answeredKeys decides how much of the outcome the press is told back.
//
// A PREFERENCE TOKEN gets the real list, which it has always had: it is the
// credential the preference centre runs on, its holder can read the whole
// consent state on the next GET anyway, and the screen uses the names.
//
// A WITHDRAWAL CREDENTIAL gets the COUNT and not the names. It is a long-lived
// bearer token deliberately not allowed to read a consent state, and the
// purpose keys are that state: an all-marketing press would otherwise
// enumerate every marketing purpose the workspace runs, to anyone holding a
// link out of a forwarded mail.
//
// WHAT IT DOES KEEP is whether anything moved, and an earlier version of this
// threw that away too. Returning an empty list for a successful first press
// made the page say "these emails were already switched off, nothing changed"
// to somebody who had just switched them off — the response is what the screen
// reads to tell a real withdrawal from a replay. Hiding the names is the
// privacy property; hiding the outcome was a bug wearing its clothes.
//
// The placeholder is opaque and constant, so a count is all a prober learns:
// that this press moved something, which they already know because they made
// it happen.
func answeredKeys(withdrawn []string, viaCredential bool) []string {
	if !viaCredential {
		return withdrawn
	}
	anonymous := make([]string, len(withdrawn))
	for i := range anonymous {
		anonymous[i] = withdrawalStoppedPlaceholder
	}
	return anonymous
}

// withdrawalStoppedPlaceholder stands in for a purpose name the presser may
// not learn. The screen counts the list rather than rendering it, so a
// constant is enough and a real key would defeat the point.
const withdrawalStoppedPlaceholder = "stopped"

// unsubscribe stops what this press asked to stop.
//
// A named purpose is taken as given — it is the one the message was sent
// under, and the mailbox provider naming it must not be second-guessed.
//
// Unnamed means "all of it", and the store decides WHICH inside the
// transaction that withdraws them. Choosing here would be a selection made in
// one transaction and acted on in another, and a purpose granted in that
// window would survive the press that reported success.
func (h Handlers) unsubscribe(
	ctx context.Context, contactID ids.ContactID, params crmcontracts.OneClickUnsubscribeParams,
	viaCredential bool,
) ([]string, error) {
	if params.Purpose != nil && strings.TrimSpace(*params.Purpose) != "" {
		named := strings.ToLower(strings.TrimSpace(*params.Purpose))
		if viaCredential {
			// THE CREDENTIAL'S SCOPE BOUNDS WHAT THE REQUEST MAY NAME. A
			// named-purpose credential has already had its own key written
			// into params by oneClickSubject, so reaching here with a
			// different one is impossible; an ALL-MARKETING credential,
			// though, would otherwise stop whatever the query string asked
			// for — including business correspondence, which is the thing the
			// class filter below exists to spare. The scope says marketing,
			// so a purpose outside that class is beyond the link's authority.
			return h.store.WithdrawMarketingNamed(ctx, contactID, named)
		}
		return h.store.PublicWithdrawAll(ctx, contactID, []string{named})
	}
	// BOTH CREDENTIAL FAMILIES STOP THE SAME THING, which is what an
	// unsubscribe means: the marketing classes and nothing else. No branch on
	// which link carried the press, because the answer no longer differs.
	//
	// It used to. The legacy sweep stopped every purpose except the locked
	// transactional one, so a press there also ended business correspondence —
	// a contact who unsubscribed from a newsletter stopped receiving replies to
	// their own enquiries. Narrowing it changes what links already sitting in
	// mailboxes do, and that is the point: those links say "unsubscribe", and
	// stopping somebody's replies was never what they offered.
	//
	// A subject who wants everything stopped has a different route, and it is a
	// different legal act: an Art. 21 objection recorded as a subject_request
	// suppression, not a withdrawal of a consent that was never the basis for
	// those messages.
	return h.store.PublicStopAllMarketing(ctx, contactID)
}

// oneClickSubject resolves the press to the contact it acts for, accepting a
// preference token OR a withdrawal credential.
//
// BOTH FAMILIES, because the whole point of the credential is that the link in
// an old message keeps working after the preference token that rode with it has
// rotated. A press is a press; which credential carried it is our bookkeeping,
// not the recipient's problem.
//
// THE CREDENTIAL'S SCOPE OVERRIDES A NAMED PURPOSE from the query string. A
// named-purpose credential may stop the one subscription it was minted for and
// nothing else, so a request naming a different purpose is refused rather than
// quietly widened — a link that stopped more than it was for would be acting
// beyond the authority the recipient was handed.
func (h Handlers) oneClickSubject(
	ctx context.Context, token string, params crmcontracts.OneClickUnsubscribeParams,
) (ids.ContactID, crmcontracts.OneClickUnsubscribeParams, bool, error) {
	if ref, err := h.store.ResolvePreferenceToken(ctx, token); err == nil {
		return ref.ContactID, params, false, nil
	}
	ref, err := h.store.ResolveWithdrawalToken(ctx, token)
	if err != nil {
		return ids.ContactID{}, params, false, err
	}
	if ref.ContactID.IsZero() {
		// A lead-only or address-only credential. Withdrawing a per-purpose
		// consent state needs a contact to hold it, and there is none — so the
		// caller records a stop rather than refusing. Answering 404 here, as
		// this did first, handed a lead a link that resolved and then said no,
		// which defeats the mint that issued it.
		return ids.ContactID{}, params, true, errNoConsentSubject
	}
	if ref.Scope == WithdrawalScopeNamedPurpose {
		named, err := h.store.purposeKeyByID(ctx, ref.PurposeID)
		if err != nil {
			return ids.ContactID{}, params, true, err
		}
		if params.Purpose != nil && !strings.EqualFold(strings.TrimSpace(*params.Purpose), named) {
			return ids.ContactID{}, params, true, &ValidationError{
				Field: fieldKeyPurpose,
				Reason: "this unsubscribe link stops one named subscription, and the request names " +
					"a different one",
			}
		}
		params.Purpose = &named
	}
	return ref.ContactID, params, true, nil
}

// errNoConsentSubject says the credential names nobody who can hold a
// per-purpose consent state. It is a routing answer inside this package, never
// a status: the press succeeds, by a different write.
var errNoConsentSubject = errors.New("consent: this link names no contact")

// stopForCredential records the press for a subject with no consent state.
//
// It answers the SAME body a contact's press answers, so the mailbox provider
// posting this cannot tell the two subjects apart and does not learn which it
// got. The list is empty because a stop is one row rather than a set of
// purposes — there are no names to count here, and the page says the recipient
// is unsubscribed either way.
func (h Handlers) stopForCredential(w http.ResponseWriter, r *http.Request, token string) {
	// The TOKEN, not the ref the caller already resolved: the store re-resolves
	// inside the transaction that writes, so an erasure committing in between
	// is decisive rather than raced. See StopForCredential.
	if err := h.store.StopForCredential(r.Context(), token); err != nil {
		writeConsentErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, map[string]any{"unsubscribed": []string{}})
}

// maxPreferenceChoices bounds a single granular save. The consent purpose
// catalog is a small closed set; anything beyond a generous ceiling is
// abuse, not a real preference update.
const maxPreferenceChoices = 64

// UpdatePreferences implements (PUT /public/preferences/{token}): the
// granular save, committed as ONE transaction. Each choice carries the
// exact wording shown, stored verbatim as proof. A grant the engine
// refuses is reported back by name rather than taking the save with it —
// see preferencesave.go for why that is not "all or nothing".
func (h Handlers) UpdatePreferences(w http.ResponseWriter, r *http.Request, token string) {
	choices, ok := decodePreferenceChoices(w, r)
	if !ok {
		return
	}
	ref, err := h.store.ResolvePreferenceToken(r.Context(), token)
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	refused, err := h.store.PublicSaveChoices(r.Context(), ref.ContactID, choices)
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	view, err := h.store.PublicPreferenceView(r.Context(), ref)
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	writePreferenceCenter(w, view, refused)
}

// decodePreferenceChoices admits the body: shape, size, states and keys,
// with a purpose named twice settled toward its withdrawal before
// anything is written.
func decodePreferenceChoices(w http.ResponseWriter, r *http.Request) ([]PreferenceChoiceInput, bool) {
	var req struct {
		Choices []struct {
			PurposeKey string  `json:"purpose_key"`
			State      string  `json:"state"`
			Wording    *string `json:"wording"`
		} `json:"choices"`
	}
	if !httperr.Decode(w, r, &req) {
		return nil, false
	}
	if len(req.Choices) == 0 {
		httperr.Write(w, r, httperr.Validation("choices", "required", "at least one per-purpose choice is required"))
		return nil, false
	}
	// A legitimate save carries at most one choice per tracked purpose;
	// the catalog is a small closed set. Cap the array so a valid token
	// cannot amplify a single 1 MiB body into tens of thousands of
	// per-choice writes.
	if len(req.Choices) > maxPreferenceChoices {
		httperr.Write(w, r, httperr.Validation("choices", "too_many", "more choices than there are tracked purposes"))
		return nil, false
	}
	out := make([]PreferenceChoiceInput, 0, len(req.Choices))
	for _, c := range req.Choices {
		state, err := ParseRecordableState(c.State)
		if err != nil {
			httperr.Write(w, r, httperr.Validation(fieldState, "invalid", "must be granted or withdrawn"))
			return nil, false
		}
		// Normalized HERE, as the engine will read it, so a duplicate
		// check cannot miss "Newsletter " and "newsletter" as a pair.
		out = append(out, PreferenceChoiceInput{
			PurposeKey: normalizedPurposeKey(c.PurposeKey),
			State:      state,
			Wording:    c.Wording,
		})
	}
	return settleTowardWithdrawal(out), true
}

// writePreferenceCenter is the one spelling of this response, so the read
// and the save cannot answer in different shapes.
//
// Held by: TestThePreferenceCentreAnswersInOneShape (backend/gates/preferencecentrewriters_test.go)
func writePreferenceCenter(w http.ResponseWriter, view PreferenceView, refused []ChoiceOutcome) {
	body := map[string]any{
		"purposes":       wirePurposeChoices(view.Purposes),
		"masked_email":   view.MaskedEmail,
		"workspace_name": view.WorkspaceName,
	}
	out := make([]map[string]any, 0, len(refused))
	for _, f := range refused {
		out = append(out, map[string]any{fieldPurposeKey: f.PurposeKey, "reason": f.Reason})
	}
	body["refused"] = out
	httperr.WriteJSON(w, http.StatusOK, body)
}

func wirePurposeChoices(choices []PurposeChoice) []map[string]any {
	out := make([]map[string]any, 0, len(choices))
	for _, c := range choices {
		out = append(out, map[string]any{
			"key":                      c.Key,
			"label":                    c.Label,
			fieldState:                 c.State,
			"locked":                   c.Locked,
			"grant_needs_confirmation": c.GrantNeedsConfirmation,
			"choice":                   string(c.Choice),
			"can_opt_in":               c.CanOptIn,
		})
	}
	return out
}

// PublicStopContact implements (POST /public/preferences/{token}/stop): the
// route for a subject who wants MORE stopped than an unsubscribe stops.
//
// A DIFFERENT LEGAL ACT from the unsubscribe beside it, which is why it is a
// different route. One-click withdraws the marketing-class purposes, which is
// what a subscription link offers. This records an Art. 21 objection, which
// says the processing must stop whether or not consent was ever its basis —
// the only thing that reaches business correspondence.
func (h Handlers) PublicStopContact(w http.ResponseWriter, r *http.Request, token string) {
	var body crmcontracts.PublicStopContactJSONRequestBody
	if !httperr.Decode(w, r, &body) {
		return
	}
	contactID, err := h.publicStopSubject(r.Context(), token)
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	statement := ""
	if body.Statement != nil {
		statement = *body.Statement
	}
	result, err := h.store.PublicStop(
		r.Context(), contactID, PublicStopAction(body.Action), statement)
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, map[string]any{
		"recorded":          result.Recorded,
		"receipt_reference": result.ReceiptReference,
	})
}

// publicStopSubject resolves the press to the contact it acts for, accepting a
// preference token OR a withdrawal credential.
//
// BOTH FAMILIES, for the reason the one-click door gives: the whole point of
// the credential is that a link in an old message keeps working after the
// preference token that rode with it has rotated. A subject reaching for the
// stronger stop is the last one who should be told their link expired.
//
// A CREDENTIAL WITH NO CONTACT IS REFUSED here, and that is narrower than the
// one-click door deliberately. There the lead-only case records an
// address-scoped stop, which suits a marketing withdrawal. An Art. 21 objection
// is about one contact's processing and is read by contact, so recording one
// against an address that belongs to no contact would write a stop nothing
// evaluates — worse than saying no, because the subject would be told it
// worked.
func (h Handlers) publicStopSubject(ctx context.Context, token string) (ids.ContactID, error) {
	if ref, err := h.store.ResolvePreferenceToken(ctx, token); err == nil {
		return ref.ContactID, nil
	}
	ref, err := h.store.ResolveWithdrawalToken(ctx, token)
	if err != nil {
		return ids.ContactID{}, err
	}
	if ref.ContactID.IsZero() {
		return ids.ContactID{}, apperrors.ErrNotFound
	}
	// THE CREDENTIAL'S SCOPE BOUNDS WHAT THE PRESS MAY ASK FOR, the same way it
	// bounds which purpose the one-click door may stop.
	//
	// A named_purpose link was minted to stop ONE subscription. NEITHER action
	// here is that narrow — the smaller one objects to all direct marketing —
	// so the link authorizes neither, and it is refused rather than widened to
	// the nearest thing it nearly covers. Honouring stop_all_contact from it
	// would let a forwarded newsletter link end that contact's invoices and
	// their replies, at the subject's own authority, which no seat can lift.
	//
	// The preference centre is where a subject proves more: its token reads and
	// writes their whole record, so a press from there carries the authority
	// this one does not.
	if ref.Scope == WithdrawalScopeNamedPurpose {
		return ids.ContactID{}, &ValidationError{
			Field: "action",
			Reason: "this link stops one named subscription, and both of these stops are " +
				"broader than it carries — use the preferences page it links to",
		}
	}
	return ref.ContactID, nil
}

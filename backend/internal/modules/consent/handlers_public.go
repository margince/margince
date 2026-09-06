// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The no-login preference-center transport (B-E11.32). The public
// middleware has already resolved the token to (workspace, person) and
// bound the workspace GUC plus the system principal; each handler
// re-resolves the token for the person id (the same infra read) and then
// drives the consent engine. An unknown or revoked token reads as absent
// (404) — the surface is never a consent-state oracle, and a GET/prefetch
// on the unsubscribe path never withdraws (only POST is routed to it).

import (
	"context"
	"net/http"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
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
	ref, err := h.store.ResolvePreferenceToken(r.Context(), token)
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	withdrawn, err := h.unsubscribe(r.Context(), ref.PersonID, params)
	if err != nil {
		writeConsentErr(w, r, err)
		return
	}
	if withdrawn == nil {
		withdrawn = []string{}
	}
	httperr.WriteJSON(w, http.StatusOK, map[string]any{"unsubscribed": withdrawn})
}

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
	ctx context.Context, personID ids.PersonID, params crmcontracts.OneClickUnsubscribeParams,
) ([]string, error) {
	if params.Purpose != nil && strings.TrimSpace(*params.Purpose) != "" {
		named := strings.ToLower(strings.TrimSpace(*params.Purpose))
		return h.store.PublicWithdrawAll(ctx, personID, []string{named})
	}
	return h.store.PublicWithdrawEverything(ctx, personID)
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
	refused, err := h.store.PublicSaveChoices(r.Context(), ref.PersonID, choices)
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

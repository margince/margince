// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

var onboardingRequiredFields = []string{fieldDisplayName, fieldOfferSummary, fieldICP}

const onboardingCompanyDraftMaxRunes = 2_000

type onboardingCompanyAssistant struct {
	state   onboardingStateReader
	people  onboardingSiteReadReader
	brain   completer
	runtime runTransparencyReader
	rollout *string
	// voice backs the voice act's deterministic context; nil means the
	// role wired no voice store and the act answers without corpus numbers.
	voice onboardingVoiceReader
	// company reports the anchor's presence for the results and connect
	// acts; nil falls back to the site read's confirmation state alone.
	company onboardingCompanyReader
}

type onboardingStateReader interface {
	Get(context.Context) (identity.OnboardingState, error)
}

type onboardingSiteReadReader interface {
	GetCompanySiteRead(context.Context, ids.UUID) (people.SiteRead, []people.SiteReadComparison, error)
}

type onboardingConversationContext struct {
	Dossier           []companyReadEvidence           `json:"dossier_evidence"`
	CurrentDraft      identity.OnboardingCompanyDraft `json:"current_company_draft"`
	NextRequired      string                          `json:"next_required_field,omitempty"`
	RemainingRequired []string                        `json:"remaining_required_fields"`
}

type onboardingResearchState struct {
	status    string
	ready     bool
	confirmed bool
}

func (a *onboardingCompanyAssistant) message(w http.ResponseWriter, r *http.Request) {
	if a == nil || a.brain == nil || a.runtime == nil {
		httperr.NotImplemented(w, r, "messageOnboardingCompany (no model path configured)")
		return
	}
	if a.rollout != nil && !companyContextOnboardingEnabled(*a.rollout) {
		httperr.NotImplemented(w, r, "messageOnboardingCompany (company onboarding disabled)")
		return
	}
	req, message, ok := decodeOnboardingCompanyMessage(w, r)
	if !ok {
		return
	}
	state, err := a.state.Get(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	history, validationErr := companyReadConversation(req.History)
	if validationErr != nil {
		httperr.Write(w, r, httperr.Validation("history", "invalid", validationErr.Error()))
		return
	}
	evidence, runID, research, read, comparisons, err := a.onboardingEvidence(r.Context(), state)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	currentDraft := state.CompanyDraft
	if req.CompanyDraft != nil {
		currentDraft = onboardingDraftInput(*req.CompanyDraft)
	}
	remaining := remainingOnboardingFields(currentDraft)
	act := onboardingRequestAct(req)
	if act != string(crmcontracts.OnboardingActCompany) {
		// The recap acts speak about the company that EXISTS, not about
		// the resumable draft: a manually saved anchor is a confirmed
		// company with no required fields left, whatever the draft says.
		present, presenceErr := a.companyPresent(r.Context(), research)
		if presenceErr != nil {
			httperr.Write(w, r, presenceErr)
			return
		}
		if present {
			research.confirmed = true
			remaining = nil
		}
	}
	conversation := onboardingConversationContext{
		Dossier: evidence, CurrentDraft: currentDraft,
		RemainingRequired: remaining,
	}
	if len(remaining) > 0 {
		conversation.NextRequired = remaining[0]
	}

	answer, clarify, actAction, err := a.converse(r.Context(), req, act, message, history, conversation, research, read, comparisons, runID)
	if err != nil {
		// A model this installation cannot reach is a DEPENDENCY that is down,
		// not a fault in the request — and httperr.Write has no sentinel for it,
		// so it would fall through to an opaque 500 whose body names nothing.
		//
		// It names the way through instead. Every required field can be typed by
		// hand, so a model outage does not actually block onboarding: it blocks
		// the assistant. Someone who is told only "internal error" has no way to
		// know that, and the wizard is the first thing they ever see.
		if modelUnreachable(err) {
			httperr.ServiceUnavailable(w, r,
				"the assistant is unavailable — check the model binding under Settings → AI. "+
					"You can enter the company details yourself in the meantime; nothing here needs the assistant")
			return
		}
		httperr.Write(w, r, err)
		return
	}
	runtime, err := a.runtime.Get(r.Context(), runID)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	response := onboardingCompanyReply(act, answer, evidence, remaining, research, runtime)
	response.Clarify = clarify
	if actAction != nil {
		response.AvailableAction = actAction
	}
	httperr.WriteJSON(w, http.StatusOK, response)
}

// converse routes the message to its act's answer path and returns the
// reply plus the deterministic attachments the act produced: the
// detected clarify question (company act) or the act's next action.
func (a *onboardingCompanyAssistant) converse(ctx context.Context, req crmcontracts.OnboardingCompanyMessageRequest, act, message string, history []model.Message, conversation onboardingConversationContext, research onboardingResearchState, read *people.SiteRead, comparisons []people.SiteReadComparison, runID ids.UUID) (companyReadModelReply, *crmcontracts.OnboardingClarify, *crmcontracts.OnboardingCompanyMessageReplyAvailableAction, error) {
	locale := string(req.Locale)
	remaining := conversation.RemainingRequired
	switch {
	case act != string(crmcontracts.OnboardingActCompany):
		voiceCtx, err := a.voiceContext(ctx)
		if err != nil {
			return companyReadModelReply{}, nil, nil, err
		}
		contextJSON, err := onboardingActContext(act, voiceCtx, a.voice != nil, research, remaining)
		if err != nil {
			return companyReadModelReply{}, nil, nil, err
		}
		answer, err := a.answerAct(principal.WithCorrelationID(ctx, runID), act, message, history, contextJSON, locale)
		if err != nil {
			return companyReadModelReply{}, nil, nil, err
		}
		return answer, nil, onboardingActAction(act, voiceCtx, a.voice != nil, research), nil
	case isCompanyStatusQuestion(message):
		return companyReadModelReply{Kind: companyConversationStatus, Message: onboardingStatusMessage(locale, research, len(remaining))}, nil, nil, nil
	default:
		if req.SelectedOption != nil {
			if err := verifySelectedOption(*req.SelectedOption, read, comparisons, locale); err != nil {
				return companyReadModelReply{}, nil, nil, err
			}
		}
		answer, err := a.answer(principal.WithCorrelationID(ctx, runID), message, history, conversation, locale, req.SelectedOption)
		if err != nil {
			return companyReadModelReply{}, nil, nil, err
		}
		// A clarification carries the first STILL-OPEN server-detected
		// question: the model's prose stays, the options are never its
		// own, and a question the current draft already answers with an
		// exact option value is not re-asked.
		var clarify *crmcontracts.OnboardingClarify
		if answer.Kind == "clarification" && read != nil {
			if questions := openOnboardingClarifies(*read, comparisons, locale, conversation.CurrentDraft); len(questions) > 0 {
				clarify = &questions[0]
			}
		}
		return answer, clarify, nil, nil
	}
}

// onboardingRequestAct resolves the request's act; absent means company.
// Validity was checked at decode time.
func onboardingRequestAct(req crmcontracts.OnboardingCompanyMessageRequest) string {
	if req.Act == nil {
		return string(crmcontracts.OnboardingActCompany)
	}
	return string(*req.Act)
}

func decodeOnboardingCompanyMessage(w http.ResponseWriter, r *http.Request) (crmcontracts.OnboardingCompanyMessageRequest, string, bool) {
	var req crmcontracts.OnboardingCompanyMessageRequest
	if !httperr.Decode(w, r, &req) {
		return req, "", false
	}
	if !req.Locale.Valid() {
		httperr.Write(w, r, httperr.Validation("locale", "invalid", onboardingLocaleRefusal()))
		return req, "", false
	}
	message := strings.TrimSpace(req.Message)
	if message == "" {
		httperr.Write(w, r, httperr.Validation("message", "empty", "write a company-setup message for Margince"))
		return req, "", false
	}
	if len([]rune(message)) > companyReadMessageMaxRunes {
		httperr.Write(w, r, httperr.Validation("message", "too_long", "message must be at most 2000 characters"))
		return req, "", false
	}
	if req.CompanyDraft != nil {
		if field := oversizedOnboardingDraftField(*req.CompanyDraft); field != "" {
			httperr.Write(w, r, httperr.Validation("company_draft."+field, "too_long", "each company draft field must be at most 2000 characters"))
			return req, "", false
		}
	}
	if req.Act != nil && !req.Act.Valid() {
		httperr.Write(w, r, httperr.Validation("act", "invalid", "act must be company, voice, results, or connect"))
		return req, "", false
	}
	if req.SelectedOption != nil {
		if field, code, detail := invalidOnboardingSelection(req); field != "" {
			httperr.Write(w, r, httperr.Validation(field, code, detail))
			return req, "", false
		}
	}
	return req, message, true
}

// invalidOnboardingSelection checks the clarify-option echo: it exists
// only in the company act, and it must name a real company field with a
// non-empty value — the pair it authorizes verbatim.
func invalidOnboardingSelection(req crmcontracts.OnboardingCompanyMessageRequest) (field, code, detail string) {
	selection := *req.SelectedOption
	if onboardingRequestAct(req) != string(crmcontracts.OnboardingActCompany) {
		return "selected_option", "invalid", "a clarify selection applies only to the company act"
	}
	if strings.TrimSpace(selection.ClarifyId) == "" {
		return "selected_option.clarify_id", "empty", "echo the clarify id the option belongs to"
	}
	if !crmcontracts.CompanySiteReadSuggestedChangeField(strings.TrimSpace(selection.Field)).Valid() {
		return "selected_option.field", "invalid", "the selection must name a known company field"
	}
	if strings.TrimSpace(selection.Value) == "" {
		return "selected_option.value", "empty", "the selection must carry the chosen value verbatim"
	}
	return "", "", ""
}

func oversizedOnboardingDraftField(draft crmcontracts.OnboardingCompanyDraft) string {
	fields := []struct {
		name  string
		value *string
	}{
		{fieldDisplayName, draft.DisplayName},
		{fieldOfferSummary, draft.OfferSummary},
		{fieldICP, draft.Icp},
		{fieldValueProposition, draft.ValueProposition},
		{fieldUSP, draft.Usp},
		{fieldCustomerPains, draft.CustomerPains},
		{fieldDesiredOutcomes, draft.DesiredOutcomes},
		{fieldBuyingCenter, draft.BuyingCenter},
		{fieldBuyingIntents, draft.BuyingIntents},
		{fieldCommonObjections, draft.CommonObjections},
		{fieldSalesMotion, draft.SalesMotion},
		{fieldLegalName, draft.LegalName},
		{fieldRegisteredAddress, draft.RegisteredAddress},
		{fieldRegisterVat, draft.RegisterVat},
		{fieldIndustry, draft.Industry},
		{fieldHistory, draft.History},
	}
	for _, field := range fields {
		if field.value != nil && len([]rune(*field.value)) > onboardingCompanyDraftMaxRunes {
			return field.name
		}
	}
	return ""
}

func (a *onboardingCompanyAssistant) onboardingEvidence(ctx context.Context, state identity.OnboardingState) ([]companyReadEvidence, ids.UUID, onboardingResearchState, *people.SiteRead, []people.SiteReadComparison, error) {
	if state.SiteReadID == nil {
		return nil, state.ID, onboardingResearchState{ready: true}, nil, nil, nil
	}
	read, comparisons, err := a.people.GetCompanySiteRead(ctx, *state.SiteReadID)
	if err != nil {
		return nil, ids.UUID{}, onboardingResearchState{}, nil, nil, err
	}
	research := onboardingResearchState{
		status:    read.Status,
		ready:     read.Status == siteReadWireStatusDone || read.Status == siteReadWireStatusPartial,
		confirmed: read.ConfirmedAt != nil,
	}
	return companyReadEvidenceSet(read), read.ID, research, &read, comparisons, nil
}

func (a *onboardingCompanyAssistant) answer(ctx context.Context, message string, history []model.Message, conversation onboardingConversationContext, locale string, selection *crmcontracts.OnboardingClarifySelection) (companyReadModelReply, error) {
	req, err := onboardingCompanyAnswerRequest(message, history, conversation, locale, selection)
	if err != nil {
		return companyReadModelReply{}, err
	}
	gate := newOnboardingCompanyGate(message, history, conversation, selection)
	var response model.Response
	if structured, ok := a.brain.(validatedBrain); ok {
		response, err = structured.CompleteValidated(ctx, req, gate.validate)
	} else {
		response, err = a.brain.Complete(ctx, req)
	}
	if err != nil {
		return companyReadModelReply{}, err
	}
	var reply companyReadModelReply
	if err := json.Unmarshal([]byte(ai.Unfence(response.Text)), &reply); err != nil {
		return companyReadModelReply{}, fmt.Errorf("compose: onboarding company answer is not valid JSON: %w", err)
	}
	if err := gate.validateReply(reply); err != nil {
		return companyReadModelReply{}, err
	}
	return reply, nil
}

func remainingOnboardingFields(draft identity.OnboardingCompanyDraft) []string {
	values := map[string]*string{
		fieldDisplayName:  draft.DisplayName,
		fieldOfferSummary: draft.OfferSummary,
		fieldICP:          draft.ICP,
	}
	remaining := make([]string, 0, len(onboardingRequiredFields))
	for _, field := range onboardingRequiredFields {
		value := values[field]
		if value == nil || strings.TrimSpace(*value) == "" {
			remaining = append(remaining, field)
		}
	}
	return remaining
}

func isCompanyStatusQuestion(message string) bool {
	normalized := strings.ToLower(strings.Join(strings.Fields(message), " "))
	normalized = strings.TrimRight(normalized, "?!. ")
	for _, phrase := range []string{
		"does this work", "does this work now", "is this working", "is this working now", "is this working yet",
		"what is the status", "what is the status now", "what is the status of the website research", "what is the status of website research",
		"funktioniert das", "funktioniert das jetzt", "klappt das", "klappt das jetzt", "wie ist der status", "wie ist jetzt der status",
		"wie ist der status der web-recherche", "wie ist der status der website-recherche",
	} {
		if normalized == phrase {
			return true
		}
	}
	return false
}

func onboardingStatusMessage(locale string, research onboardingResearchState, missing int) string {
	said := copyFor(locale)
	switch {
	case research.confirmed:
		return said.statusConfirmed
	case research.status == siteReadWireStatusFailed:
		return fmt.Sprintf(said.statusFailed, missing)
	case !research.ready:
		return said.statusResearching
	default:
		return fmt.Sprintf(said.statusMissing, missing)
	}
}

func onboardingCompanyReply(act string, answer companyReadModelReply, evidence []companyReadEvidence, remaining []string, research onboardingResearchState, runtime ai.RunSummary) crmcontracts.OnboardingCompanyMessageReply {
	base := contractCompanyReadReply(answer, evidence, runtime)
	out := crmcontracts.OnboardingCompanyMessageReply{
		Kind: base.Kind, Message: base.Message, Act: crmcontracts.OnboardingAct(act),
		ProposedChanges: base.ProposedChanges,
		Citations:       base.Citations, RemainingRequiredFields: make([]crmcontracts.OnboardingCompanyMessageReplyRemainingRequiredFields, len(remaining)),
		AiRuntime: base.AiRuntime,
	}
	for i, field := range remaining {
		out.RemainingRequiredFields[i] = crmcontracts.OnboardingCompanyMessageReplyRemainingRequiredFields(field)
	}
	if act != string(crmcontracts.OnboardingActCompany) {
		return out
	}
	if len(remaining) > 0 {
		next := crmcontracts.OnboardingCompanyMessageReplyNextRequiredField(remaining[0])
		out.NextRequiredField = &next
	} else if research.ready && !research.confirmed {
		action := crmcontracts.OnboardingAvailableActionConfirmCompany
		out.AvailableAction = &action
	}
	return out
}

func (h onboardingStateHandlers) MessageOnboardingCompany(w http.ResponseWriter, r *http.Request) {
	if h.assistant == nil {
		httperr.NotImplemented(w, r, "messageOnboardingCompany (no model path configured)")
		return
	}
	h.assistant.message(w, r)
}

// modelUnreachable reports whether err is the model lane failing rather than
// this request being wrong.
//
// Matched on ai.ErrAllTiersFailed, the aggregate the router raises once the
// walk has reached the end of the bound rungs: the one place that distinction
// is already made, and a sentinel rather than the message text, which would be
// a second copy of it.
func modelUnreachable(err error) bool {
	return errors.Is(err, ai.ErrAllTiersFailed)
}

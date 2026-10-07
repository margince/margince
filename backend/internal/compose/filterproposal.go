// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Proposing a filter from plain words (POST /filters/propose).
//
// It lives in compose for filter preview's reason and one more: it reads the
// collections store's vocabulary, the custom-field catalogue's labels and the
// nl_search model lane, and only compose may hold all three. What it answers is
// never saved — the builder shows the proposal as ordinary clauses, the preview
// counts it, and a human presses Save.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/filterpropose"
	"github.com/margince/margince/backend/internal/compose/modelfailure"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// filterProposalMaxRunes is the contract's maxLength for the sentence: a list
// description, not a document to be summarised.
const filterProposalMaxRunes = 500

// codeAINotConfigured answers a deployment with no model bound: there is no
// deterministic reading of a sentence to fall back to, so it says so instead.
const codeAINotConfigured = "ai_not_configured"

// customFieldLabels answers the admin's label for each cf_* column, keyed by
// column name. A reader without custom_field:read gets none and the model reads
// the column names alone.
type customFieldLabels func(ctx context.Context, resource string) (map[string]string, error)

// filterProposalHandlers shadows the generated ProposeFilter stub.
type filterProposalHandlers struct {
	pool        *pgxpool.Pool
	collections *collections.Store
	labels      customFieldLabels
	// lane is the nl_search model lane, nil in a role that wires no model.
	lane completer
	now  func() time.Time
}

// filterProposalResponse is FilterProposal with the tree typed: the generated
// type holds it as a map, and the gate already answers a storekit.Predicate.
type filterProposalResponse struct {
	Resource    crmcontracts.FilterProposalResource      `json:"resource"`
	Filter      *storekit.Predicate                      `json:"filter"`
	Unsupported []crmcontracts.FilterProposalUnsupported `json:"unsupported"`
	ModelUsed   *string                                  `json:"model_used,omitempty"`
}

// withFilterProposalLane binds the nl_search lane, the WithRoleLane shape.
func (h filterProposalHandlers) withFilterProposalLane(lane completer) filterProposalHandlers {
	h.lane = lane
	return h
}

func (h filterProposalHandlers) ProposeFilter(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.RequireHuman(ctx); err != nil {
		httperr.Write(w, r, err)
		return
	}
	var req crmcontracts.FilterProposalRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	text, err := proposalText(req)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	resource := string(req.Resource)
	// Read on the record type as well as on lists: a proposal is only worth its
	// model call to somebody who can then preview what it selects.
	if err := auth.Require(ctx, resource, principal.ActionRead); err != nil {
		httperr.Write(w, r, err)
		return
	}
	if h.lane == nil {
		httperr.Write(w, r, &httperr.DetailedError{
			Status: http.StatusConflict, Code: codeAINotConfigured,
			Detail: "plain-words filters need an AI model — an administrator can bind one under Settings → AI; the filter can still be built by hand",
		})
		return
	}
	fields, err := h.vocabulary(ctx, resource)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	in := filterpropose.Input{
		Resource: resource, Text: text, Fields: fields,
		Today: h.now().UTC(), Lang: h.language(ctx, req.Locale),
	}
	ask, err := withCompanyContextIfReadable(ctx, filterpropose.Request(in))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	res, err := ai.Ask(ctx, h.lane, ask, func(reply string) error {
		_, err := filterpropose.Parse(reply)
		return err
	})
	if err != nil {
		modelfailure.Write(w, r, err)
		return
	}
	answer, err := filterpropose.Parse(res.Text)
	if err != nil {
		modelfailure.Write(w, r, fmt.Errorf("%w: %w", ai.ErrOutputRejected, err))
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireProposal(req.Resource, filterpropose.Gate(answer, fields), res.ServedModel))
}

// withCompanyContextIfReadable asks for our offer and market only when the
// caller may read the company it is read from. The context helps a sentence like
// "our target market" and is never needed, so a caller without company read
// still gets a proposal rather than the context read's 403.
func withCompanyContextIfReadable(ctx context.Context, req model.Request) (model.Request, error) {
	err := auth.Require(ctx, "company", principal.ActionRead)
	switch {
	case err == nil:
		req.IncludeCompanyContext = true
	case !errors.Is(err, apperrors.ErrPermissionDenied):
		return model.Request{}, err
	}
	return req, nil
}

// proposalText is the sentence, trimmed and bounded the way the contract
// declares, refused with the field named.
func proposalText(req crmcontracts.FilterProposalRequest) (string, error) {
	if !req.Resource.Valid() {
		return "", httperr.Validation("resource", "invalid_enum", "resource must be one of contact, company, deal, lead")
	}
	if req.Locale != nil && !req.Locale.Valid() {
		return "", httperr.Validation("locale", "invalid_enum", "locale must be one of en, de, vi")
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return "", httperr.Validation("text", codeInvalid, "describe the list in a few words")
	}
	if utf8.RuneCountInString(text) > filterProposalMaxRunes {
		return "", httperr.Validation("text", codeInvalid,
			fmt.Sprintf("a description is at most %d characters", filterProposalMaxRunes))
	}
	return text, nil
}

// vocabulary is the caller's own filter vocabulary, the one the builder offers,
// with each custom field's label beside its column.
func (h filterProposalHandlers) vocabulary(ctx context.Context, resource string) ([]filterpropose.Field, error) {
	offered, ok, err := h.collections.FilterVocabulary(ctx, resource)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, apperrors.ErrNotFound
	}
	labels, err := h.labels(ctx, resource)
	if err != nil {
		return nil, err
	}
	fields := make([]filterpropose.Field, 0, len(offered))
	for _, f := range offered {
		fields = append(fields, filterpropose.Field{
			Name: f.Name, Type: f.Type, Operators: f.Operators, Options: f.Options,
			Custom: f.Custom, Label: labels[f.Name], Currency: f.Currency,
		})
	}
	return fields, nil
}

// language is the reader's own interface language when the client sent it, and
// the installation's base language otherwise.
func (h filterProposalHandlers) language(ctx context.Context, locale *crmcontracts.FilterProposalRequestLocale) string {
	if locale != nil {
		return string(*locale)
	}
	return identity.BaseLanguageForPrompt(ctx, h.pool)
}

func wireProposal(
	resource crmcontracts.FilterProposalRequestResource, proposal filterpropose.Proposal, served string,
) filterProposalResponse {
	out := filterProposalResponse{
		Resource:    crmcontracts.FilterProposalResource(resource),
		Filter:      proposal.Tree,
		Unsupported: make([]crmcontracts.FilterProposalUnsupported, 0, len(proposal.Unsupported)),
	}
	for _, dropped := range proposal.Unsupported {
		item := crmcontracts.FilterProposalUnsupported{
			Phrase: dropped.Phrase, Code: crmcontracts.FilterProposalUnsupportedCode(dropped.Code), Reason: dropped.Reason,
		}
		if dropped.Field != "" {
			field := dropped.Field
			item.Field = &field
		}
		out.Unsupported = append(out.Unsupported, item)
	}
	if served != "" {
		out.ModelUsed = &served
	}
	return out
}

// catalogLabels reads the labels from the custom-field catalogue under the
// catalogue's own grant. A denial is an answer — the fields are still offered,
// under their column names — and anything else is the caller's to see.
func catalogLabels(catalog *customfields.Service) customFieldLabels {
	return func(ctx context.Context, resource string) (map[string]string, error) {
		active, limit := string(crmcontracts.CustomFieldStatusActive), 200
		rows, _, err := catalog.List(ctx, customfields.ListInput{Object: resource, Status: &active, Limit: &limit})
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return map[string]string{}, nil
		}
		if err != nil {
			return nil, err
		}
		labels := make(map[string]string, len(rows))
		for _, row := range rows {
			if row.ColumnName != nil {
				labels[*row.ColumnName] = row.Label
			}
		}
		return labels, nil
	}
}

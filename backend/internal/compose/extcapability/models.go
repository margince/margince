// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package extcapability is the core's half of what an extension borrows: its AI
// models and its object store. The compose root supplies the gate that checks a
// unit's declaration and the lane or store behind it. This package holds the
// translation and the limits, so the root package stays small.
package extcapability

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
	"github.com/margince/margince/backend/pkg/extension"
)

// ErrModelsUnwired is a role that bound no model lane.
var ErrModelsUnwired = errors.New("extension: this process has no AI model bound")

// ErrModelFailed is what a unit sees of a failed model call. The router's own
// text can name a provider, a model or a budget figure.
var ErrModelFailed = errors.New("extension: the installation's model could not answer")

// Lane is the one model call a unit's request reaches.
type Lane interface {
	Complete(ctx context.Context, req model.Request) (model.Response, error)
}

// Models is extension.Models over a lane. Gate checks the unit's declaration and
// returns the invocation's context. Trace writes the audit row before the call.
type Models struct {
	Lane  Lane
	Unit  string
	Gate  func(context.Context) (context.Context, error)
	Trace func(context.Context) error
}

// Complete answers one request through the lane.
func (m Models) Complete(ctx context.Context, req extension.CompleteRequest) (string, error) {
	ctx, err := m.Gate(ctx)
	if err != nil {
		return "", err
	}
	modelReq, err := Request(req)
	if err != nil {
		return "", err
	}
	if m.Lane == nil {
		return "", ErrModelsUnwired
	}
	if err := m.Trace(ctx); err != nil {
		return "", err
	}
	// Asked again after the trace write, the slow step before the call, so a
	// declaration removed meanwhile stops the call before it spends.
	if _, err := m.Gate(ctx); err != nil {
		return "", err
	}
	resp, err := m.Lane.Complete(ctx, modelReq)
	switch {
	case errors.Is(err, model.ErrSubjectRestricted):
		return "", fmt.Errorf("%w: a contact this request carries is under a restriction of processing", extension.ErrForbidden)
	case err != nil:
		slog.ErrorContext(ctx, "extcapability: an extension model call failed", "err", err, "unit", m.Unit)
		return "", ErrModelFailed
	}
	return resp.Text, nil
}

// Trace writes the system_log row that names the unit beside the call's
// correlation id, which the router's call record carries too. It commits before
// the call, so a unit cannot spend and roll the trace away.
func Trace(ctx context.Context, pool *pgxpool.Pool, unit string) error {
	detail := map[string]any{"extension": unit, "task": string(ai.TaskExtensionCall)}
	if cid, ok := principal.CorrelationID(ctx); ok {
		detail["correlation_id"] = cid.String()
	}
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		_, err := storekit.LogSystem(ctx, tx, "extension_model_call", detail)
		return err
	})
	if err != nil {
		slog.ErrorContext(ctx, "extcapability: an extension model call could not be traced", "err", err, "unit", unit)
		return ErrModelFailed
	}
	return nil
}

// Request is a unit's request as the router takes it. The task honours
// restrictions of processing. A request that names neither its records nor why
// it holds none is refused here, with a message the unit can act on.
//
//promptlang:exempt the unit writes the whole prompt and the core adds none, so there is no core sentence for a language rule to apply to; the unit's own text decides the language.
//promptvoice:exempt the unit writes the whole prompt and the core adds none, so there is no Margince voice to compose into it.
func Request(req extension.CompleteRequest) (model.Request, error) {
	if len(req.Messages) == 0 {
		return model.Request{}, fmt.Errorf("%w: a model request needs at least one message", extension.ErrInvalid)
	}
	messages := make([]model.Message, 0, len(req.Messages))
	for _, msg := range req.Messages {
		if msg.Role != "user" && msg.Role != "assistant" {
			return model.Request{}, fmt.Errorf("%w: a message role is user or assistant, not %q", extension.ErrInvalid, msg.Role)
		}
		messages = append(messages, model.Message{Role: msg.Role, Content: msg.Text})
	}
	if req.MaxTokens < 0 {
		return model.Request{}, fmt.Errorf("%w: MaxTokens is not negative", extension.ErrInvalid)
	}
	subjects, err := subjectsOf(req)
	if err != nil {
		return model.Request{}, err
	}
	return model.Request{System: req.System, Messages: messages, MaxTokens: req.MaxTokens, Subjects: subjects}, nil
}

func subjectsOf(req extension.CompleteRequest) (*model.Subjects, error) {
	if len(req.About) == 0 {
		if strings.TrimSpace(req.NoContactData) == "" {
			return nil, fmt.Errorf("%w: name the records the text was built from in About, or say in NoContactData why it holds no contact's data", extension.ErrInvalid)
		}
		return model.AboutNoContact(req.NoContactData), nil
	}
	var subjects model.Subjects
	for _, ref := range req.About {
		id, err := ids.Parse(ref.ID)
		if err != nil {
			return nil, fmt.Errorf("%w: the %s id is not a canonical UUID", extension.ErrInvalid, ref.Type)
		}
		slot := map[string]*[]ids.UUID{
			"contact": &subjects.Contacts, "company": &subjects.Companies, "deal": &subjects.Deals,
			"activity": &subjects.Activities, "project": &subjects.Projects, "lead": &subjects.Leads,
		}[ref.Type]
		if slot == nil {
			return nil, fmt.Errorf("%w: About names contact, company, deal, activity, project or lead records, not %q", extension.ErrInvalid, ref.Type)
		}
		if !slices.Contains(*slot, id) {
			*slot = append(*slot, id)
		}
	}
	return &subjects, nil
}

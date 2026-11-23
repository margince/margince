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

// Models is extension.Models over the lanes of the declared tasks. Gate checks
// the unit's declaration and returns the invocation's context. Trace writes the
// audit row before the call. Unattended is true for a job tick or a bus delivery,
// which may call a background task only.
type Models struct {
	Lane       func(ai.Task) Lane
	Unit       string
	Unattended bool
	Gate       func(context.Context) (context.Context, error)
	Trace      func(context.Context, ai.Task) error
}

// Complete answers one request through the lane of one declared task.
func (m Models) Complete(ctx context.Context, name string, req extension.CompleteRequest) (string, error) {
	ctx, err := m.Gate(ctx)
	if err != nil {
		return "", err
	}
	task, err := m.admit(name)
	if err != nil {
		return "", err
	}
	modelReq, err := Request(req)
	if err != nil {
		return "", err
	}
	tier, err := TierPin(req)
	if err != nil {
		return "", err
	}
	ctx = ai.WithTierPin(ctx, tier)
	ctx = withSubject(ctx, task, req)
	var lane Lane
	if m.Lane != nil {
		lane = m.Lane(task)
	}
	if lane == nil {
		return "", ErrModelsUnwired
	}
	if err := m.Trace(ctx, task); err != nil {
		return "", err
	}
	// Asked again after the trace write, the slow step before the call, so a
	// declaration removed meanwhile stops the call before it spends.
	if _, err := m.Gate(ctx); err != nil {
		return "", err
	}
	resp, err := lane.Complete(ctx, modelReq)
	switch {
	case errors.Is(err, model.ErrSubjectRestricted):
		return "", fmt.Errorf("%w: a contact this request carries is under a restriction of processing", extension.ErrForbidden)
	case errors.Is(err, model.ErrAttachmentUnsupported):
		return "", fmt.Errorf("%w: the model for this tier cannot read one of the attachments; ask for another tier", extension.ErrInvalid)
	case err != nil:
		slog.ErrorContext(ctx, "extcapability: an extension model call failed", "err", err, "unit", m.Unit, "task", name)
		return "", ErrModelFailed
	}
	return resp.Text, nil
}

// admit finds the task the unit named among the tasks it declared. A task of
// another unit or of the core answers the same as an unknown one, so a unit
// cannot learn which tasks exist.
func (m Models) admit(name string) (ai.Task, error) {
	task := ai.Task(name)
	if unit, ok := ai.ExtensionTaskUnit(task); !ok || unit != m.Unit {
		return "", fmt.Errorf("%w: declare the task %q in the unit's api/ai-tasks.yaml", extension.ErrNotDeclared, name)
	}
	if m.Unattended && !ai.IsBackground(task) {
		return "", fmt.Errorf("%w: a job tick or a bus delivery calls background tasks only, and %q is interactive", extension.ErrForbidden, name)
	}
	return task, nil
}

// withSubject names the record on the call log when the request names one. A
// request that names several records, or none, leaves the subject empty.
func withSubject(ctx context.Context, task ai.Task, req extension.CompleteRequest) context.Context {
	if len(req.About) == 0 {
		slog.InfoContext(ctx, "extcapability: an extension model call holds no contact data",
			"task", string(task), "reason", req.NoContactData)
		return ctx
	}
	if len(req.About) != 1 {
		return ctx
	}
	id, err := ids.Parse(req.About[0].ID)
	if err != nil {
		return ctx
	}
	return ai.WithSubject(ctx, ids.Ref{Type: req.About[0].Type, ID: id}, "")
}

// Trace writes the system_log row that names the unit beside the call's
// correlation id, which the router's call record carries too. It commits before
// the call, so a unit cannot spend and roll the trace away.
func Trace(ctx context.Context, pool *pgxpool.Pool, unit string, task ai.Task) error {
	detail := map[string]any{"extension": unit, "task": string(task)}
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
	files, err := attachmentsOf(req.Attachments)
	if err != nil {
		return model.Request{}, err
	}
	return model.Request{
		System: req.System, Messages: messages, MaxTokens: req.MaxTokens, Subjects: subjects,
		Attachments: files, ResponseSchema: req.JSONSchema,
	}, nil
}

// TierPin is the router rung a unit's request is pinned to; empty keeps the
// task's whole ladder.
func TierPin(req extension.CompleteRequest) (string, error) { return tierOf(req.Tier) }

// tierOf names the router's rung for the tier a unit asked for.
func tierOf(tier extension.ModelTier) (string, error) {
	switch tier {
	case extension.ModelTierDefault:
		return "", nil
	case extension.ModelTierCheap:
		return string(ai.TierCheapCloud), nil
	case extension.ModelTierPremium:
		return string(ai.TierPremium), nil
	}
	return "", fmt.Errorf("%w: Tier is cheap or premium, not %q", extension.ErrInvalid, string(tier))
}

// attachmentsOf checks a unit's files before any byte reaches a provider.
func attachmentsOf(files []extension.Attachment) ([]model.Attachment, error) {
	if len(files) > extension.MaxAttachments {
		return nil, fmt.Errorf("%w: a request carries at most %d attachments", extension.ErrInvalid, extension.MaxAttachments)
	}
	var out []model.Attachment
	for _, file := range files {
		if !slices.Contains([]string{"image/png", "image/jpeg", "image/webp", "image/gif", "application/pdf"}, file.ContentType) {
			return nil, fmt.Errorf("%w: an attachment is an png, jpeg, webp or gif image or a PDF, not %q", extension.ErrInvalid, file.ContentType)
		}
		if len(file.Data) == 0 || len(file.Data) > extension.MaxAttachmentBytes {
			return nil, fmt.Errorf("%w: an attachment holds 1 byte to %d bytes", extension.ErrInvalid, extension.MaxAttachmentBytes)
		}
		out = append(out, model.Attachment{MIME: file.ContentType, Bytes: file.Data, Name: file.Name})
	}
	return out, nil
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

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The core's half of Runtime.AI: a unit's declared AI task runs through the
// same router as the product's own tasks, as the one `extension_task` task.
// Routing, the region limit, credential redaction, the budget, the restriction
// of processing and the ai_call row all come from that path. This file adds
// what is the unit's own: the declaration check, the document lanes and the
// answer check.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/pdftext"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
	"github.com/margince/margince/backend/pkg/extension"
)

// maxExtensionAIInputChars bounds the text one call sends: the free text and
// every document's text together, in characters. It is the bound the core's own
// document reading holds a text lane to.
const maxExtensionAIInputChars = maxDocumentTextChars

// maxExtensionAIDocuments bounds how many documents one call may carry.
const maxExtensionAIDocuments = 8

var errExtensionAIUnwired = errors.New("compose: this role bound no AI path for extensions")

// BindExtensionAI records the model lane a unit's AI calls ride. A role that
// runs extension work calls it once at boot with ModelPath.ExtensionTask. A role
// that never calls it answers ErrAIUnavailable to every AI call.
func BindExtensionAI(lane extensionAILane) {
	extensionRuntimeDeps.mu.Lock()
	defer extensionRuntimeDeps.mu.Unlock()
	extensionRuntimeDeps.ai = lane
}

// extensionAILane is the router lane of the `extension_task` task.
type extensionAILane interface {
	completer
	CompleteValidated(ctx context.Context, req model.Request, validate ai.Validator) (model.Response, error)
}

// AI is the unit's AI port for this call, guarded by this Runtime's lifetime.
//
//nolint:ireturn // the published port, as Secrets.
func (r *callRuntime) AI() extension.AI { return callAI{rt: r} }

type callAI struct{ rt *callRuntime }

func (a callAI) Run(ctx context.Context, name extension.AITaskName, in extension.AIInput) (extension.AIResult, error) {
	ctx, err := a.rt.scoped(ctx)
	if err != nil {
		return extension.AIResult{}, err
	}
	task, ok := declaredAITask(a.rt.unit, name)
	if !ok {
		return extension.AIResult{}, extension.ErrAITaskUndeclared
	}
	lane := a.rt.deps.ai
	if lane == nil {
		return extension.AIResult{}, fmt.Errorf("%w: %w", extension.ErrAIUnavailable, errExtensionAIUnwired)
	}
	req, err := extensionAIRequest(ctx, task, in)
	if err != nil {
		return extension.AIResult{}, err
	}
	validate := extensionAnswerCheck(task)
	resp, err := ai.Ask(ai.WithExtensionTask(ctx, string(task.Name)), lane, req, validate)
	if err != nil {
		return extension.AIResult{}, extensionAIError(ctx, err)
	}
	// Checked again here: a lane without the retry policy does not run it.
	if err := validate(resp.Text); err != nil {
		return extension.AIResult{}, fmt.Errorf("%w: the model's answer did not fit the task's schema", extension.ErrAIUnavailable)
	}
	out := extension.AIResult{Text: resp.Text, MachineMade: true}
	if task.Schema != "" {
		out.JSON = json.RawMessage(resp.Text)
	}
	return out, nil
}

// declaredAITask finds the task the named unit declared in this boot's set.
func declaredAITask(unit string, name extension.AITaskName) (extension.AITask, bool) {
	for _, ext := range ComposedExtensions() {
		if string(ext.Name) != unit {
			continue
		}
		for _, task := range ext.AITasks {
			if task.Name == name {
				return task, true
			}
		}
	}
	return extension.AITask{}, false
}

// extensionAIRequest builds the one request a call sends. The unit's prompt is
// the instruction; everything the call carries is data inside a fence whose
// marker only this call knows. A part is identified by its position, which this
// system chooses, and never by a filename the unit or a sender wrote.
//
//promptlang:exempt the unit's declared prompt owns the answer's language; the core adds only the data boundary.
//promptvoice:exempt the answer goes back to the unit's code, not to a reader, so no voice applies.
func extensionAIRequest(ctx context.Context, task extension.AITask, in extension.AIInput) (model.Request, error) {
	subjects, err := extensionAISubjects(in)
	if err != nil {
		return model.Request{}, err
	}
	texts, err := extensionAIDocumentTexts(ctx, task, in.Documents)
	if err != nil {
		return model.Request{}, err
	}
	parts := texts
	if strings.TrimSpace(in.Text) != "" {
		parts = append([]string{in.Text}, texts...)
	}
	size := 0
	for _, part := range parts {
		size += utf8.RuneCountInString(part)
	}
	if size > maxExtensionAIInputChars {
		return model.Request{}, fmt.Errorf("%w: the input is %d characters, at most %d are read in one call",
			extension.ErrInvalid, size, maxExtensionAIInputChars)
	}
	fence := promptfence.New()
	var user strings.Builder
	for i, part := range parts {
		user.WriteString(fence.WrapAttr("part", strconv.Itoa(i+1), part) + "\n")
	}
	req := model.Request{
		System:   task.Prompt + "\n" + fence.Rule("input"),
		Messages: []model.Message{{Role: "user", Content: user.String()}},
		Subjects: subjects,
	}
	if task.Schema != "" {
		req.ResponseSchema = json.RawMessage(task.Schema)
	}
	return req, nil
}

// extensionAISubjects is the contacts a call carries, as the router's
// restriction check needs them. A call must name them or say it has none: a
// restriction of processing cannot be checked against an input nobody described.
func extensionAISubjects(in extension.AIInput) (*model.Subjects, error) {
	if len(in.Contacts) > 0 {
		subjects := model.SubjectsOf(in.Contacts, nil)
		if subjects.Empty() {
			return nil, fmt.Errorf("%w: Contacts holds no valid contact id", extension.ErrInvalid)
		}
		return subjects, nil
	}
	if strings.TrimSpace(in.NoContact) == "" {
		return nil, fmt.Errorf("%w: name the contacts the call carries, or say in NoContact why it carries none", extension.ErrInvalid)
	}
	return model.AboutNoContact(in.NoContact), nil
}

// extensionAIDocumentTexts turns each document into the text the model is given.
// Text types are their own text. A PDF goes through the reader the core reads
// its own PDFs with, and a scan is refused rather than sent unread. No document
// is ever sent to a vendor as bytes.
func extensionAIDocumentTexts(ctx context.Context, task extension.AITask, docs []extension.AIDocument) ([]string, error) {
	if len(docs) == 0 {
		return nil, nil
	}
	if !task.Documents {
		return nil, fmt.Errorf("%w: AI task %q does not declare Documents", extension.ErrInvalid, task.Name)
	}
	if len(docs) > maxExtensionAIDocuments {
		return nil, fmt.Errorf("%w: %d documents, at most %d per call", extension.ErrInvalid, len(docs), maxExtensionAIDocuments)
	}
	out := make([]string, 0, len(docs))
	for _, doc := range docs {
		if len(doc.Body) > maxDocumentBytes {
			return nil, fmt.Errorf("%w: a document is over %d MiB", extension.ErrInvalid, maxDocumentBytes>>20)
		}
		mime := mediaTypeOf(doc.ContentType)
		switch {
		case model.CarriesMIME(documentTextMIMEs, mime):
			out = append(out, strings.TrimSpace(string(doc.Body)))
		case mime == documentPDFMIME:
			text, err := processPDFReader.Read(ctx, doc.Body, maxExtensionAIInputChars)
			switch {
			case errors.Is(err, pdftext.ErrNoTextLayer):
				return nil, extension.ErrAIScannedDocument
			case err != nil:
				return nil, fmt.Errorf("%w: the PDF could not be read", extension.ErrAIDocumentType)
			}
			out = append(out, text)
		default:
			return nil, extension.ErrAIDocumentType
		}
	}
	return out, nil
}

// extensionAnswerCheck is the answer's own check, fed back to the model on a
// retry. A task with a schema must answer a JSON value that conforms to it.
func extensionAnswerCheck(task extension.AITask) ai.Validator {
	if task.Schema == "" {
		return func(text string) error {
			if strings.TrimSpace(text) == "" {
				return errors.New("the answer is empty")
			}
			return nil
		}
	}
	return func(text string) error {
		schema, err := compileExtensionSchema(task.Schema)
		if err != nil {
			return err
		}
		answer, err := jsonschema.UnmarshalJSON(strings.NewReader(text))
		if err != nil {
			return errors.New("the answer is not valid JSON")
		}
		if err := schema.Validate(answer); err != nil {
			return fmt.Errorf("the answer does not fit the schema: %w", err)
		}
		return nil
	}
}

func compileExtensionSchema(src string) (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(strings.NewReader(src))
	if err != nil {
		return nil, fmt.Errorf("the task's schema is not valid JSON: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("task.json", doc); err != nil {
		return nil, fmt.Errorf("the task's schema was refused: %w", err)
	}
	schema, err := compiler.Compile("task.json")
	if err != nil {
		return nil, fmt.Errorf("the task's schema was refused: %w", err)
	}
	return schema, nil
}

// extensionAIError answers a failed call in the unit's vocabulary. It names
// the reasons a unit can act on. Anything else is logged here and reaches the
// unit as a plain failure, so a provider's words never travel to a screen.
func extensionAIError(ctx context.Context, err error) error {
	switch {
	case ctx.Err() != nil:
		return ctx.Err()
	case errors.Is(err, model.ErrSubjectRestricted):
		return fmt.Errorf("%w: a contact the call carries is under a restriction of processing", extension.ErrAIUnavailable)
	case errors.Is(err, ai.ErrBudgetDeferred):
		return fmt.Errorf("%w: the AI budget for this period is spent", extension.ErrAIUnavailable)
	case errors.Is(err, ai.ErrAllTiersFailed), errors.Is(err, httperr.ErrNotConfigured):
		return fmt.Errorf("%w: no model is set up that this installation's routing and region limit allow for this task", extension.ErrAIUnavailable)
	}
	slog.Default().WarnContext(ctx, "extension AI call failed", "error", err)
	return fmt.Errorf("%w: the model call failed", extension.ErrAIUnavailable)
}

// preflightAITasks runs the published Validate the generator also runs, and
// refuses a name the unit declares twice. Nothing applies until the whole set
// has passed.
func preflightAITasks(e extension.Extension) error {
	seen := map[extension.AITaskName]bool{}
	for _, task := range e.AITasks {
		if err := task.Validate(); err != nil {
			return fmt.Errorf("compose: extension %q: %w", e.Name, err)
		}
		if seen[task.Name] {
			return fmt.Errorf("compose: extension %q declares AI task %q twice", e.Name, task.Name)
		}
		seen[task.Name] = true
		if task.Schema != "" {
			if _, err := compileExtensionSchema(task.Schema); err != nil {
				return fmt.Errorf("compose: extension %q: AI task %q: %w", e.Name, task.Name, err)
			}
		}
	}
	return nil
}

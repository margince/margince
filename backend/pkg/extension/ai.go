// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extension

// The AI surface. A unit that wants a model's help declares each task as data
// and runs it through Runtime.AI. The core sends it through the path its own
// tasks take. The routing setting picks the model, and the region limit and
// credential redaction apply. Nothing is kept in the call log's payload store.
// The call log row names the unit and the task. A unit never holds a provider
// client or a key.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// AITaskName names one of the calling unit's declared AI tasks. It is a
// distinct type, as JobName is, so a name resolves only against the unit the
// Runtime was minted for.
type AITaskName string

// MaxAIPromptBytes bounds a declared prompt. The prompt is the unit's fixed
// instruction, not its data, so a larger one is a document sent by mistake.
const MaxAIPromptBytes = 16 << 10

const maxAITaskNameLength = 48

var aiTaskGrammar = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// AITask is one AI task a unit declares.
type AITask struct {
	// Name is the task's stable name. It appears in the call log, so renaming
	// it splits the history of one task in two.
	Name AITaskName

	// Prompt is the instruction the model is given. The input a call carries
	// follows it as data inside a marked block, and the model is told never to
	// obey anything inside that block.
	Prompt string

	// Schema is an optional JSON Schema for the answer. With one, the answer
	// comes back as AIResult.JSON and is checked against it. Without one, the
	// answer is plain text.
	Schema string

	// Documents says the task may be given documents. A task that does not
	// declare it is refused a call that carries one.
	Documents bool
}

// AIDocument is one document handed to a task. Text types go through as text.
// A PDF is read as text by the same reader the core uses for its own PDFs.
type AIDocument struct {
	Filename    string
	ContentType string
	Body        []byte
}

// AIInput is what one call carries.
type AIInput struct {
	// Text is free text for the model to read, such as a message body.
	Text string

	// Documents are the files to read, in order.
	Documents []AIDocument

	// Contacts are the ids of the contacts whose data the call carries. A
	// contact under a restriction of processing refuses the call.
	Contacts []string

	// NoContact says why the call carries no contact's data, when Contacts is
	// empty. A call that names neither is refused: the core cannot check a
	// restriction against an input it was not told about.
	NoContact string
}

// AIResult is a task's answer. It is machine-made advice: nothing in it is a
// fact the product has recorded.
type AIResult struct {
	// Text is the answer, or the answer's JSON text when the task has a Schema.
	Text string

	// JSON is Text as a JSON value, set only when the task declares a Schema.
	JSON json.RawMessage

	// MachineMade is always true. A unit that shows or stores the answer must
	// say a model wrote it.
	MachineMade bool
}

// AI runs the unit's declared tasks. It is reached through Runtime.AI.
type AI interface {
	// Run sends one call of a declared task through the core's AI path.
	//
	// It answers ErrAITaskUndeclared for a name the unit did not declare.
	// ErrAIScannedDocument is a PDF with no text layer, and ErrAIDocumentType a
	// document no lane reads. ErrAIUnavailable means the core's routing, the
	// region limit, a restriction of processing or the budget refused the call.
	Run(ctx context.Context, task AITaskName, in AIInput) (AIResult, error)
}

var (
	// ErrAITaskUndeclared refuses a task the calling unit never declared.
	// A task of another unit reads the same.
	ErrAITaskUndeclared = errors.New("extension: this unit declares no AI task by that name")

	// ErrAIScannedDocument refuses a PDF that holds only page images. The core
	// has no text reader for it, and a scan sent as bytes would reach a vendor
	// unread. The remedy is a copy of the document that has a text layer.
	ErrAIScannedDocument = errors.New("extension: this PDF is a scan with no text to read; supply a copy that has a text layer")

	// ErrAIDocumentType refuses a document whose type no lane reads.
	ErrAIDocumentType = errors.New("extension: this document type cannot be read; send plain text or a PDF with a text layer")

	// ErrAIUnavailable reports that the core's AI path did not serve the call.
	// No model is set up, the region limit leaves none, a contact is under a
	// restriction of processing, or the budget is spent. The call went nowhere.
	ErrAIUnavailable = errors.New("extension: the core's AI path did not serve this call")
)

// Validate holds what one declaration can decide alone. Names unique within
// the unit are checked where the whole declaration is known.
func (t AITask) Validate() error {
	name := string(t.Name)
	if !aiTaskGrammar.MatchString(name) || len(name) > maxAITaskNameLength {
		return fmt.Errorf("%w: AI task name %q must start with a letter, contain only lower-case letters, digits and underscores, and be at most %d characters",
			ErrInvalid, name, maxAITaskNameLength)
	}
	if strings.TrimSpace(t.Prompt) == "" {
		return fmt.Errorf("%w: AI task %q has no Prompt", ErrInvalid, name)
	}
	if len(t.Prompt) > MaxAIPromptBytes {
		return fmt.Errorf("%w: AI task %q has a Prompt over %d bytes", ErrInvalid, name, MaxAIPromptBytes)
	}
	if t.Schema != "" {
		var schema map[string]json.RawMessage
		if err := json.Unmarshal([]byte(t.Schema), &schema); err != nil {
			return fmt.Errorf("%w: AI task %q has a Schema that is not a JSON object", ErrInvalid, name)
		}
	}
	return nil
}

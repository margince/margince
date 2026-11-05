// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"encoding/json"
	"errors"

	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// shownRefusal is a staged refusal that also has an answer to show. It unwraps
// to the staged refusal, so the runner still suspends on it and a task still
// waits on it. The dispatcher serves the sealed answer as the refusal's
// structured content, which a view draws its choice from.
type shownRefusal struct {
	staged *workflow.StagedApprovalError
	// answer is the handler's payload, and sealed is that payload in the
	// result envelope, which the registry seals as it seals a success.
	answer json.RawMessage
	sealed json.RawMessage
}

func (e *shownRefusal) Error() string { return e.staged.Error() }

func (e *shownRefusal) Unwrap() error { return e.staged }

// sealShown seals the answer a refusal carries and answers the refusal itself.
// A seal that fails is joined to it, so the staged refusal still reaches what
// waits on it.
func sealShown(err error, seal func(json.RawMessage) (json.RawMessage, error)) error {
	var shown *shownRefusal
	if !errors.As(err, &shown) {
		return err
	}
	sealed, sealErr := seal(shown.answer)
	if sealErr != nil {
		return errors.Join(err, sealErr)
	}
	shown.sealed = sealed
	return err
}

// shownAnswer answers the sealed answer a refusal carries, or nil.
func shownAnswer(err error) json.RawMessage {
	var shown *shownRefusal
	if !errors.As(err, &shown) {
		return nil
	}
	return shown.sealed
}

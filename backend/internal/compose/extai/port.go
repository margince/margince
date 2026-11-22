// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package extai is the core's half of Runtime.AI. A unit's declared AI task
// runs through the same router as the product's own tasks, as the one
// `extension_task` task. Routing, the region limit, credential redaction, the
// budget, the restriction check and the ai_call row all come from that path.
// This package adds what is the unit's own: the declaration check, the document
// lanes and the answer check. The compose package wires it to a Runtime.
package extai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/pdftext"
	"github.com/margince/margince/backend/internal/shared/ports/model"
	"github.com/margince/margince/backend/pkg/extension"
)

// Lane is the router lane of the `extension_task` task.
type Lane interface {
	Complete(ctx context.Context, req model.Request) (model.Response, error)
	CompleteValidated(ctx context.Context, req model.Request, validate ai.Validator) (model.Response, error)
}

var bound struct {
	mu   sync.RWMutex
	pool *pgxpool.Pool
	lane Lane
}

// Bind records the model lane a unit's AI calls ride, with the pool the role
// runs on. A role that runs extension work calls it once at boot with
// ModelPath.ExtensionTask. A port on another pool finds no lane, so a lane built
// over a replaced pool is never used. A role that never calls it answers
// ErrAIUnavailable to every call.
func Bind(pool *pgxpool.Pool, lane Lane) {
	bound.mu.Lock()
	defer bound.mu.Unlock()
	bound.pool, bound.lane = pool, lane
}

// laneFor answers the lane bound for pool, or nil.
//
//nolint:ireturn // the bound lane is an interface by design; tests substitute it.
func laneFor(pool *pgxpool.Pool) Lane {
	bound.mu.RLock()
	defer bound.mu.RUnlock()
	if bound.pool != pool {
		return nil
	}
	return bound.lane
}

// Port is one unit's AI port for one invocation.
type Port struct {
	// Scope pins the call to the invocation's tenant and unit, and refuses a
	// Runtime that has been released.
	Scope func(context.Context) (context.Context, error)
	// Unit is the calling unit, and Declared lists the composed set it is found in.
	Unit     string
	Declared func() []extension.Extension
	// Lane overrides the bound lane in tests.
	Lane Lane
	// Restricted answers whether any contact is under a restriction of
	// processing. Nil reads the workspace through Pool.
	Restricted func(context.Context) (bool, error)
	Pool       *pgxpool.Pool
	// PDF is the core's one PDF reader.
	PDF *pdftext.Reader
	// MaxDocumentBytes and MaxInputChars are the bounds the core's own document
	// reading holds, passed in so the two readings share one number.
	MaxDocumentBytes int
	MaxInputChars    int
}

var _ extension.AI = Port{}

var errUnwired = errors.New("extai: this role bound no AI path for extensions")

// Run sends one call of a declared task through the router.
func (p Port) Run(ctx context.Context, name extension.AITaskName, in extension.AIInput) (extension.AIResult, error) {
	ctx, err := p.Scope(ctx)
	if err != nil {
		return extension.AIResult{}, err
	}
	task, ok := p.task(name)
	if !ok {
		return extension.AIResult{}, extension.ErrAITaskUndeclared
	}
	lane := p.Lane
	if lane == nil {
		lane = laneFor(p.Pool)
	}
	if lane == nil {
		return extension.AIResult{}, fmt.Errorf("%w: %w", extension.ErrAIUnavailable, errUnwired)
	}
	req, err := p.request(ctx, task, in)
	if err != nil {
		return extension.AIResult{}, err
	}
	if err := p.refuseWhileAContactIsRestricted(ctx); err != nil {
		return extension.AIResult{}, err
	}
	validate := answerCheck(task)
	resp, err := ai.Ask(ai.WithExtensionTask(ctx, string(task.Name)), lane, req, validate)
	if err != nil {
		return extension.AIResult{}, callError(ctx, err)
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

// task finds a task the calling unit declared.
func (p Port) task(name extension.AITaskName) (extension.AITask, bool) {
	for _, ext := range p.Declared() {
		if i := slices.IndexFunc(ext.AITasks, func(t extension.AITask) bool { return t.Name == name }); i >= 0 && string(ext.Name) == p.Unit {
			return ext.AITasks[i], true
		}
	}
	return extension.AITask{}, false
}

// refuseWhileAContactIsRestricted refuses every call in a workspace that holds
// a contact under a restriction of processing.
// The core cannot see where a unit's input came from, so the unit's own
// Contacts and NoContact statements are not what the check rests on.
// The router still checks the named contacts.
func (p Port) refuseWhileAContactIsRestricted(ctx context.Context) error {
	check := p.Restricted
	if check == nil {
		check = func(ctx context.Context) (bool, error) {
			restricted := false
			err := database.WithWorkspaceTx(ctx, p.Pool, func(tx pgx.Tx) error {
				var err error
				restricted, err = storekit.AnyContactRestricted(ctx, tx)
				return err
			})
			return restricted, err
		}
	}
	restricted, err := check(ctx)
	if err != nil {
		slog.Default().WarnContext(ctx, "extension AI call could not read restrictions", "error", err)
		return fmt.Errorf("%w: restrictions of processing could not be read", extension.ErrAIUnavailable)
	}
	if restricted {
		return fmt.Errorf("%w: a contact is under a restriction of processing, and an extension's input cannot be checked against it",
			extension.ErrAIUnavailable)
	}
	return nil
}

// callError answers a failed call in the unit's vocabulary. It names the
// reasons a unit can act on. Anything else is logged here and reaches the unit
// as a plain failure, so a provider's words never travel to a screen.
func callError(ctx context.Context, err error) error {
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

// Preflight runs the published Validate the generator also runs, and refuses a
// name the unit declares twice . Nothing
// applies until the whole set has passed.
func Preflight(e extension.Extension) error {
	seen := map[extension.AITaskName]bool{}
	for _, task := range e.AITasks {
		if err := task.Validate(); err != nil {
			return fmt.Errorf("compose: extension %q: %w", e.Name, err)
		}
		if seen[task.Name] {
			return fmt.Errorf("compose: extension %q declares AI task %q twice", e.Name, task.Name)
		}
		seen[task.Name] = true
	}
	return nil
}

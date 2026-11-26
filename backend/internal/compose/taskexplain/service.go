// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package taskexplain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/briefevidence"
	"github.com/margince/margince/backend/internal/compose/claims"
	"github.com/margince/margince/backend/internal/compose/contact360"
	"github.com/margince/margince/backend/internal/compose/contactbrief"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/generatedsummary"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ActivityReader is the activities store's own gated read of one activity.
type ActivityReader interface {
	GetActivity(ctx context.Context, id ids.ActivityID, archived storekit.ArchivedFilter) (crmcontracts.Activity, error)
}

// Assembler is the contact page's composite read, run as the caller.
type Assembler interface {
	AssembleScoped(ctx context.Context, contactID ids.ContactID, opts contact360.AssembleOptions) (crmcontracts.Contact360, error)
}

// Service writes and caches task explanations.
type Service struct {
	pool           *pgxpool.Pool
	activities     ActivityReader
	view           Assembler
	lane           Completer
	routingVersion func() string
	now            func() time.Time
	emailRows      briefevidence.Reader
}

// NewService binds the reads an explanation is written from. lane may be nil:
// the floor is then the answer.
func NewService(
	pool *pgxpool.Pool, activities ActivityReader, view Assembler, lane Completer,
	routingVersion func() string, now func() time.Time,
) *Service {
	if routingVersion == nil {
		routingVersion = func() string { return "" }
	}
	return &Service{pool: pool, activities: activities, view: view, lane: lane, routingVersion: routingVersion, now: now}
}

// WithEmailSummaries binds the reader that opens a cited message for this reader.
func (s *Service) WithEmailSummaries(reader briefevidence.Reader) *Service {
	s.emailRows = reader
	return s
}

// storedVersion is the cached payload's shape; another one reads as a miss.
const storedVersion = 1

type stored struct {
	Fingerprint string                 `json:"-"`
	Version     int                    `json:"version"`
	GeneratedAt time.Time              `json:"generated_at"`
	GeneratedBy crmcontracts.WrittenBy `json:"generated_by"`
	Sentences   []claims.Sentence      `json:"sentences"`
}

// Get serves one task's explanation, rewriting it when its input moved.
func (s *Service) Get(ctx context.Context, taskID ids.ActivityID) (crmcontracts.TaskExplanation, error) {
	if err := auth.RequireHumanOrItsAgent(ctx); err != nil {
		return crmcontracts.TaskExplanation{}, err
	}
	reader, err := actingUser(ctx)
	if err != nil {
		return crmcontracts.TaskExplanation{}, err
	}
	in, err := s.assemble(ctx, taskID)
	if err != nil {
		return crmcontracts.TaskExplanation{}, err
	}
	lang := identity.BaseLanguageForPrompt(ctx, s.pool)
	fingerprint, err := Fingerprint(in, s.routingVersion(), lang)
	if err != nil {
		return crmcontracts.TaskExplanation{}, err
	}
	cached, found, err := s.cached(ctx, reader, taskID)
	if err != nil {
		return crmcontracts.TaskExplanation{}, err
	}
	if found && cached.Version == storedVersion && cached.Fingerprint == fingerprint {
		return s.wire(ctx, taskID, cached)
	}
	lane, withheld := ai.ReadLane(ctx, s.lane)
	sentences, by, laneFailed := Write(ai.WithSubject(ctx, taskID.Ref(), in.Task.Title), lane, in, lang)
	written := stored{
		Fingerprint: fingerprint, Version: storedVersion,
		GeneratedAt: s.now().UTC(), GeneratedBy: by, Sentences: sentences,
	}
	if !laneFailed && !withheld {
		if err := s.save(ctx, reader, taskID, written); err != nil {
			return crmcontracts.TaskExplanation{}, err
		}
	}
	return s.wire(ctx, taskID, written)
}

// assemble reads the task, its source and its contact, each through the
// caller's own gates. A source or contact the caller may not open is left out.
func (s *Service) assemble(ctx context.Context, taskID ids.ActivityID) (Input, error) {
	task, err := s.activities.GetActivity(ctx, taskID, storekit.LiveOnly)
	if err != nil {
		return Input{}, err
	}
	if task.Kind != crmcontracts.ActivityKindTask {
		return Input{}, fmt.Errorf("only a task has an explanation: %w", apperrors.ErrNotFound)
	}
	in := Input{Task: FromTask(task, s.now())}
	if task.SourceActivityId != nil {
		source, err := s.activities.GetActivity(ctx, ids.From[ids.ActivityKind](ids.UUID(*task.SourceActivityId)), storekit.IncludeArchived)
		switch {
		case err == nil:
			in.Source, in.SourceText = FromSource(source)
		case !unreadable(err):
			return Input{}, err
		}
	}
	if contactID, ok := taskContact(task); ok {
		view, err := s.view.AssembleScoped(ctx, contactID, contact360.AssembleOptions{WrittenByHumanOnly: true})
		switch {
		case err == nil:
			relationship := contactbrief.FromView(view)
			in.Relationship = &relationship
		case !unreadable(err):
			return Input{}, err
		}
	}
	return in, nil
}

func unreadable(err error) bool {
	return errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied)
}

// taskContact is the contact the task is with: its contact link, else the
// contact it offers to write to.
func taskContact(task crmcontracts.Activity) (ids.ContactID, bool) {
	if task.Links != nil {
		for _, link := range *task.Links {
			if link.EntityType == crmcontracts.ActivityLinkEntityTypeContact {
				return ids.From[ids.ContactKind](ids.UUID(link.EntityId)), true
			}
		}
	}
	if target := task.EmailTarget; target != nil && target.EntityType == crmcontracts.ActivityEmailTargetEntityTypeContact {
		return ids.From[ids.ContactKind](ids.UUID(target.EntityId)), true
	}
	return ids.ContactID{}, false
}

func (s *Service) wire(ctx context.Context, taskID ids.ActivityID, written stored) (crmcontracts.TaskExplanation, error) {
	out := crmcontracts.TaskExplanation{
		TaskId:      openapi_types.UUID(taskID.UUID),
		GeneratedAt: written.GeneratedAt,
		GeneratedBy: written.GeneratedBy,
		Sentences:   wireSentences(written.Sentences),
	}
	if err := briefevidence.Attach(ctx, s.emailRows, briefevidence.FromSentences(out.Sentences)); err != nil {
		return crmcontracts.TaskExplanation{}, err
	}
	return out, nil
}

// wireSentences drops a whole sentence when any citation is not an id, so no
// claim stands on part of its evidence.
func wireSentences(in []claims.Sentence) []crmcontracts.CompanyBriefSentence {
	out := make([]crmcontracts.CompanyBriefSentence, 0, len(in))
	for _, sentence := range in {
		evidence := make([]crmcontracts.CompanyBriefEvidence, 0, len(sentence.Evidence))
		for _, cited := range sentence.Evidence {
			parsed, err := ids.Parse(cited.EntityID)
			if err != nil {
				evidence = nil
				break
			}
			evidence = append(evidence, crmcontracts.CompanyBriefEvidence{
				EntityId:   openapi_types.UUID(parsed),
				EntityType: crmcontracts.CompanyBriefEvidenceEntityType(cited.EntityType),
			})
		}
		if len(evidence) == 0 {
			continue
		}
		wired := crmcontracts.CompanyBriefSentence{Text: sentence.Text, Evidence: evidence}
		if sentence.Nature != "" {
			nature := crmcontracts.CompanyBriefSentenceNature(sentence.Nature)
			wired.Nature = &nature
		}
		out = append(out, wired)
	}
	return out
}

func (s *Service) cached(ctx context.Context, reader ids.UserID, taskID ids.ActivityID) (stored, bool, error) {
	row, found, err := generatedsummary.ReadTaskExplanation(ctx, s.pool, reader, taskID)
	if err != nil || !found {
		return stored{}, false, err
	}
	out := stored{Fingerprint: row.Fingerprint}
	if err := json.Unmarshal(row.Payload, &out); err != nil {
		//nolint:nilerr // an unreadable cache entry is a miss; the caller rewrites it
		return stored{}, false, nil
	}
	return out, true, nil
}

func (s *Service) save(ctx context.Context, reader ids.UserID, taskID ids.ActivityID, written stored) error {
	payload, err := json.Marshal(written)
	if err != nil {
		return fmt.Errorf("encode the task explanation: %w", err)
	}
	return database.WithWorkspaceTx(ctx, s.pool, func(tx pgx.Tx) error {
		return generatedsummary.SaveTaskExplanationTx(ctx, tx, reader, taskID, generatedsummary.Entry{
			Fingerprint: written.Fingerprint,
			GeneratedAt: written.GeneratedAt,
			GeneratedBy: string(written.GeneratedBy),
			Payload:     payload,
		})
	})
}

// actingUser is the human the explanation is cached for. It is written from
// what that human may see, so a call with no user has none.
func actingUser(ctx context.Context) (ids.UserID, error) {
	p, ok := principal.Actor(ctx)
	if !ok || p.UserID == (ids.UUID{}) {
		return ids.UserID{}, fmt.Errorf("a task explanation is per reader and this call carries no user: %w",
			apperrors.ErrPermissionDenied)
	}
	return ids.From[ids.UserKind](p.UserID), nil
}

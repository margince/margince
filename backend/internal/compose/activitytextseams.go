// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/readerrendering"
	"github.com/margince/margince/backend/internal/compose/rewrite"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

type (
	readerRenderingHandlers = readerrendering.Handlers
	rewriteHandlers         = rewrite.Handlers
)

// newReaderRenderingService reads the text through the activities store's
// content read, the one that refuses a row outside the caller's audience.
func newReaderRenderingService(pool *pgxpool.Pool) *readerrendering.Service {
	return readerrendering.NewService(pool, activities.NewStore(InstallationDB(pool)), time.Now)
}

// WithReaderRendering binds the reader_rendering lane. Unbound, the endpoint
// answers 409 ai_not_configured.
func WithReaderRendering(brain completer) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		s.readerRenderingHandlers = readerrendering.NewHandlers(newReaderRenderingService(pool).WithLane(brain))
		s.authHandlers = s.WithReaderRenderingAvailable(brain != nil)
	}
}

// newRewriteHandlers binds the one rewrite path, behind the composer's verbs
// and Polish wording alike, to the record read below.
func newRewriteHandlers(pool *pgxpool.Pool, brain completer) rewrite.Handlers {
	service := rewrite.NewService(brain, rewriteAbout(pool)).WithVoice(ai.NewVoiceStore(InstallationDB(pool)))
	return rewrite.NewHandlers(service)
}

// rewriteAbout admits the record a rewrite names only when the caller may read
// it, live and existing, and answers whose data it carries. A message counts
// only when its content is readable, and a lead as itself and its address.
func rewriteAbout(pool *pgxpool.Pool) rewrite.Admit {
	leads := contacts.NewStore(InstallationDB(pool))
	return func(ctx context.Context, ref ids.Ref) (*model.Subjects, error) {
		if ref.Type == (ids.LeadID{}).EntityType() {
			return leadSubjects(ctx, leads, ids.From[ids.LeadKind](ref.ID))
		}
		about := documentSubjects(ref)
		if about.Empty() {
			return nil, apperrors.ErrNotFound
		}
		if err := auth.Require(ctx, ref.Type, principal.ActionRead); err != nil {
			return nil, err
		}
		return about, database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
			if ref.Type == (ids.ActivityID{}).EntityType() {
				return auth.EnsureActivityContentVisibleLive(ctx, tx, ref.ID)
			}
			return auth.EnsureVisibleLive(ctx, tx, ref.Type, ref.ID)
		})
	}
}

// leadSubjects names a lead itself and its address, which a restriction on a
// contact holding the same address covers. The lead read is the gate.
func leadSubjects(ctx context.Context, leads *contacts.Store, id ids.LeadID) (*model.Subjects, error) {
	lead, err := leads.GetLead(ctx, id, storekit.LiveOnly)
	if err != nil {
		return nil, err
	}
	about := &model.Subjects{Leads: []ids.UUID{id.UUID}}
	if lead.Email != nil && *lead.Email != "" {
		about.Addresses = []string{string(*lead.Email)}
	}
	return about, nil
}

// WithRewrite binds the rewrite path to the drafting lane, the one the
// composer's drafts ride. Unbound, the endpoint answers 409 ai_not_configured.
func WithRewrite(brain completer) Option {
	return func(s *Server, pool *pgxpool.Pool) {
		s.rewriteHandlers = newRewriteHandlers(pool, brain)
	}
}

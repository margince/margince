// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/compose/homecache"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WithHomeCache serves a reader's first worklist page from c while it holds.
func (h Handlers) WithHomeCache(c *homecache.Cache) Handlers {
	h.home = c
	return h
}

// worklistQuestion is one worklist request, as the handler parsed it.
type worklistQuestion struct {
	view, scope, filter string
	owner               ids.UUID
	limit               int
	token               string
	omitRelationships   bool
}

// worklistAnswer is the page q asks for. A reader's own first page is served
// from the cache while it holds. A later page resumes a walk, and another
// contact's queue is not this reader's answer, so both are read fresh.
func (h Handlers) worklistAnswer(ctx context.Context, svc *Service, q worklistQuestion) (crmcontracts.Worklist, error) {
	reader, cacheable := homecache.ReaderOf(ctx)
	if h.home == nil || !cacheable || q.token != "" || (!q.owner.IsZero() && q.owner != reader) {
		return svc.WorklistView(ctx, q.view, q.scope, q.filter, q.owner, q.limit, q.token)
	}
	// The read's own entry gate, asked before a cached answer is served.
	if err := auth.RequireMember(ctx); err != nil {
		return crmcontracts.Worklist{}, err
	}
	key, err := svc.homeKey(ctx, q)
	if err != nil {
		return crmcontracts.Worklist{}, err
	}
	if out, hit, err := homecache.Lookup[crmcontracts.Worklist](ctx, h.home, reader, homecache.Worklist, key); err != nil || hit {
		return out, err
	}
	stamp, err := h.home.Stamp(ctx)
	if err != nil {
		return crmcontracts.Worklist{}, err
	}
	out, err := svc.WorklistView(ctx, q.view, q.scope, q.filter, q.owner, q.limit, q.token)
	if err != nil {
		return out, err
	}
	walk, started, err := walkStartedBy(out)
	if err != nil {
		return crmcontracts.Worklist{}, err
	}
	var walkID *ids.UUID
	if started {
		walkID = &walk
	}
	if err := homecache.Store(ctx, h.home, reader, homecache.Worklist, key, stamp, walkID, out); err != nil {
		return crmcontracts.Worklist{}, err
	}
	return out, nil
}

// homeKey names q as the service answers it, so two spellings of one question
// share one answer. It names the installation's day too, which no answer outlives.
func (s *Service) homeKey(ctx context.Context, q worklistQuestion) (string, error) {
	view, err := resolveView(q.view, q.filter)
	if err != nil {
		return "", err
	}
	scope := q.scope
	if scope == "" {
		scope = scopeMine
	}
	day, err := s.startOfDay(ctx, s.now())
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("day=%s&view=%s&scope=%s&filter=%s&own_owner=%t&limit=%d&omit_relationships=%t",
		day.Format(time.DateOnly), view, scope, q.filter, !q.owner.IsZero(), pageLimit(q.limit), q.omitRelationships), nil
}

// walkStartedBy is the walk a first page's cursor resumes, so a cached page is
// never served once its walk can no longer be continued.
func walkStartedBy(out crmcontracts.Worklist) (ids.UUID, bool, error) {
	if out.NextCursor == nil {
		return ids.UUID{}, false, nil
	}
	// Read as minted, not as decodeCursor asks: the page was minted under the
	// question the service resolved, which the request may spell differently.
	cursor, err := storekit.DecodeOpaque[worklistCursor](*out.NextCursor)
	if err != nil {
		return ids.UUID{}, false, fmt.Errorf("attention: reading the walk a page started: %w", err)
	}
	id, started := snapshotOf(cursor)
	return id, started, nil
}

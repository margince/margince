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

// worklistAnswer is the page q asks for. Home's own first page is served from
// the cache while it holds, and every other question is read fresh. A later
// page resumes a walk. Another contact's queue is not this reader's answer.
func (h Handlers) worklistAnswer(ctx context.Context, svc *Service, q worklistQuestion) (crmcontracts.Worklist, error) {
	fresh := func(ctx context.Context) (crmcontracts.Worklist, error) {
		return svc.WorklistView(ctx, q.view, q.scope, q.filter, q.owner, q.limit, q.token)
	}
	reader, cacheable := homecache.ReaderOf(ctx)
	if h.home == nil || !cacheable || q.token != "" || (!q.owner.IsZero() && q.owner != reader) {
		return fresh(ctx)
	}
	// The read's own entry gate, asked before a cached answer is served.
	if err := auth.RequireMember(ctx); err != nil {
		return crmcontracts.Worklist{}, err
	}
	key, home, err := svc.homeKey(ctx, q)
	if err != nil || !home {
		return fresh(ctx)
	}
	return homecache.Serve(ctx, h.home, reader, homecache.Worklist, key, fresh, walkStartedBy)
}

// homeKey names Home's own question as the service answers it, so the spellings
// of it share one answer, and reports whether q is that question. The key also
// names the installation's zone and day, which no answer outlives.
func (s *Service) homeKey(ctx context.Context, q worklistQuestion) (string, bool, error) {
	view, err := resolveView(q.view, q.filter)
	if err != nil {
		return "", false, err
	}
	scope := q.scope
	if scope == "" {
		scope = scopeMine
	}
	if view != viewToday || scope != scopeMine || (q.filter != "" && q.filter != string(crmcontracts.WorklistFilterAll)) || !q.omitRelationships {
		return "", false, nil
	}
	loc, err := s.location(ctx)
	if err != nil {
		return "", false, err
	}
	day := storekit.StartOfDay(s.now(), loc)
	return fmt.Sprintf("zone=%s&day=%s&own_owner=%t&limit=%d",
		loc.String(), day.Format(time.DateOnly), !q.owner.IsZero(), pageLimit(q.limit)), true, nil
}

// walkStartedBy is the walk a first page's cursor resumes, so a cached page is
// never served once its walk can no longer be continued.
func walkStartedBy(out crmcontracts.Worklist) (*ids.UUID, error) {
	if out.NextCursor == nil {
		return nil, nil //nolint:nilnil // no cursor is a page that started no walk, which Store reads as nil
	}
	// Read as minted, not as decodeCursor asks: the page was minted under the
	// question the service resolved, which the request may spell differently.
	cursor, err := storekit.DecodeOpaque[worklistCursor](*out.NextCursor)
	if err != nil {
		return nil, fmt.Errorf("attention: reading the walk a page started: %w", err)
	}
	if id, started := snapshotOf(cursor); started {
		return &id, nil
	}
	return nil, nil //nolint:nilnil // a cursor with no walk is an offset page, which Store reads as nil
}

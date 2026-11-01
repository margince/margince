// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/compose/homecache"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
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
	if h.home == nil || !cacheable || q.token != "" || !q.owner.IsZero() {
		return svc.WorklistView(ctx, q.view, q.scope, q.filter, q.owner, q.limit, q.token)
	}
	// The read's own entry gate, asked before a cached answer is served.
	if err := auth.RequireMember(ctx); err != nil {
		return crmcontracts.Worklist{}, err
	}
	key := fmt.Sprintf("view=%s&scope=%s&filter=%s&limit=%d&omit_relationships=%t",
		q.view, q.scope, q.filter, q.limit, q.omitRelationships)
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
	walk, started, err := walkStartedBy(out, q)
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

// walkStartedBy is the walk a first page's cursor resumes, so a cached page is
// never served once its walk can no longer be continued.
func walkStartedBy(out crmcontracts.Worklist, q worklistQuestion) (ids.UUID, bool, error) {
	if out.NextCursor == nil {
		return ids.UUID{}, false, nil
	}
	cursor, err := decodeCursor(*out.NextCursor, q.scope, q.filter, q.owner)
	if err != nil {
		return ids.UUID{}, false, fmt.Errorf("attention: reading the walk a page started: %w", err)
	}
	id, started := snapshotOf(cursor)
	return id, started, nil
}

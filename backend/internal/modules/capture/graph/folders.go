// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

// The mailbox's folder tree, which the capture-exclusion picker offers so
// nobody has to look up a Graph folder id by hand.

import (
	"context"
	"net/url"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// folderListMaxPages bounds the folder walk. Generous against any real mailbox
// — a hundred folders a page — so reaching it means the provider is not ending
// the list rather than that somebody has that many folders. The budget is
// spent across the WHOLE walk, children included, so a deep tree cannot turn
// one listing into an unbounded crawl.
const folderListMaxPages = 20

// folderDepthLimit bounds how far the walk descends. Outlook nests a few
// levels in practice; past this the names are longer than a picker can show
// anyway, and the bound keeps a pathological tree from spending the whole
// page budget on depth.
const folderDepthLimit = 5

// foldersPage is one /me/mailFolders response.
type foldersPage struct {
	Value []struct {
		ID          string `json:"id"`
		DisplayName string `json:"displayName"` //nolint:tagliatelle // Microsoft's wire format; must match to decode
		// ChildFolderCount saves a request per leaf: a folder that says it has
		// no children is not asked for them.
		ChildFolderCount int `json:"childFolderCount"` //nolint:tagliatelle // Microsoft's wire format; must match to decode
	} `json:"value"`
	NextLink string `json:"@odata.nextLink"` //nolint:tagliatelle // Microsoft's wire format; must match to decode
}

// ListFolders returns the mailbox's folders, nested ones included.
//
// /me/mailFolders lists only what sits directly under the root, and the folder
// somebody wants kept out of capture is usually NOT there — it is under Inbox,
// which is where a mail client puts a folder you make while reading. A listing
// of the root alone offers everything except the folders somebody actually
// wants.
//
// So the walk descends. A child's name is qualified with its parents'
// ("Inbox/Private"), which is both what the owner reads in Outlook and what
// tells two folders of the same name apart; the stored rule still names the
// provider's own id, so a later rename does not break it.
//
// Bounded two ways: folderListMaxPages across the whole walk, and
// folderDepthLimit on how deep it goes. Hitting either returns what was read
// rather than an error, because a long list that stops is more useful to a
// picker than no list at all.
func (a *httpAPI) ListFolders(ctx context.Context, accessToken string) ([]connector.NamedContainer, bool, error) {
	type pending struct {
		url    string
		prefix string
		depth  int
	}
	q := url.Values{paramSelect: {"id,displayName,childFolderCount"}, "$top": {"100"}}
	queue := []pending{{url: a.base + "/me/mailFolders?" + q.Encode()}}

	var out []connector.NamedContainer
	// Set where the walk gives up on a branch rather than finishes it: a child
	// folder past the depth limit, or a page the budget could not buy. Both
	// leave folders unlisted, and a picker that did not say so would tell
	// somebody their folder does not exist.
	truncated := false
	budget := folderListMaxPages
	for len(queue) > 0 && budget > 0 {
		cur := queue[0]
		queue = queue[1:]
		budget--

		var page foldersPage
		if _, err := a.get(ctx, accessToken, cur.url, nil, &page); err != nil {
			return nil, false, err
		}
		for _, f := range page.Value {
			if f.ID == "" {
				continue
			}
			name := f.DisplayName
			if name == "" {
				name = f.ID
			}
			qualified := cur.prefix + name
			out = append(out, connector.NamedContainer{ID: f.ID, Name: qualified})
			if f.ChildFolderCount > 0 && cur.depth >= folderDepthLimit {
				truncated = true
			}
			if f.ChildFolderCount > 0 && cur.depth < folderDepthLimit {
				queue = append(queue, pending{
					url:    a.base + "/me/mailFolders/" + url.PathEscape(f.ID) + "/childFolders?" + q.Encode(),
					prefix: qualified + "/",
					depth:  cur.depth + 1,
				})
			}
		}
		if page.NextLink == "" {
			continue
		}
		// The same origin check every stored Graph link gets: a nextLink is a
		// URL the provider chose, and following one off-origin would send this
		// mailbox's token somewhere Microsoft did not.
		if err := a.sameAPIOrigin(page.NextLink); err != nil {
			return nil, false, err
		}
		// The next page of the SAME level, so it keeps this level's prefix and
		// depth, and goes to the front: finishing a folder's own pages before
		// descending keeps the budget on breadth, where the recognisable
		// names are.
		queue = append([]pending{{url: page.NextLink, prefix: cur.prefix, depth: cur.depth}}, queue...)
	}
	// A queue with work left means the budget ran out, not that the mailbox
	// ended.
	return out, truncated || len(queue) > 0, nil
}

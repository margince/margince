// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graph

// The Graph calls behind the deletion proof (deletions.go). All four read
// with Mail.Read, the permission the connection already holds.

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
)

// folderDeletions is Microsoft's well-known name for Recoverable Items
// Deletions. Only Shift+Delete or emptying Deleted Items puts mail there.
// A move between folders, a move to the Online Archive and an archive policy
// never do.
const folderDeletions = "recoverableitemsdeletions"

// deletionsSelect is what the Deletions reads ask for: the id, and the
// internetMessageId that finds the message again after a restore changed it.
const deletionsSelect = "id,internetMessageId"

// The bounds of the fallback listing: at most this many ids per cycle.
const (
	deletionsListPageSize = 100
	deletionsListMaxPages = 5
	findMessageCopies     = 10
)

// DeletionsDelta walks the Recoverable Items Deletions delta. An empty link
// opens a fresh round over the whole folder. A 410 returns ErrDeltaGone.
func (a *httpAPI) DeletionsDelta(ctx context.Context, accessToken, deltaLink string, visit PageVisitor) (string, error) {
	start := deltaLink
	if start == "" {
		q := url.Values{paramSelect: {deletionsSelect}}
		start = a.base + "/me/mailFolders/" + folderDeletions + "/messages/delta?" + q.Encode()
	}
	if err := a.sameAPIOrigin(start); err != nil {
		return "", err
	}
	return a.deltaWalk(ctx, accessToken, start, nil, visit)
}

// ListDeletions lists the messages Recoverable Items Deletions holds, at most
// deletionsListMaxPages pages. It is the fallback when the folder's delta
// fails.
func (a *httpAPI) ListDeletions(ctx context.Context, accessToken string) ([]MessageRef, error) {
	q := url.Values{paramSelect: {deletionsSelect}, paramTop: {strconv.Itoa(deletionsListPageSize)}}
	next := a.base + "/me/mailFolders/" + folderDeletions + "/messages?" + q.Encode()
	var listed []MessageRef
	for range deletionsListMaxPages {
		var out struct {
			Value []struct {
				ID                string `json:"id"`
				InternetMessageID string `json:"internetMessageId"` //nolint:tagliatelle // Microsoft's wire format; must match to decode
			} `json:"value"`
			NextLink string `json:"@odata.nextLink"` //nolint:tagliatelle // Microsoft's wire format; must match to decode
		}
		if _, err := a.get(ctx, accessToken, next, nil, &out); err != nil {
			return nil, err
		}
		for _, m := range out.Value {
			if m.ID != "" {
				listed = append(listed, MessageRef{ID: m.ID, InternetMessageID: m.InternetMessageID})
			}
		}
		if out.NextLink == "" {
			break
		}
		if err := a.sameAPIOrigin(out.NextLink); err != nil {
			return nil, err
		}
		next = out.NextLink
	}
	return listed, nil
}

// MessageFolder reports the folder one message is filed in. found is false on
// a 404. The caller asks only about a message seen arriving in Deletions, and
// asks FindMessage before it reads a 404 as a deletion.
func (a *httpAPI) MessageFolder(ctx context.Context, accessToken, msgID string) (string, bool, error) {
	var out struct {
		ParentFolderID string `json:"parentFolderId"` //nolint:tagliatelle // Microsoft's wire format; must match to decode
	}
	q := url.Values{paramSelect: {"parentFolderId"}}
	status, err := a.get(ctx, accessToken, a.base+"/me/messages/"+url.PathEscape(msgID)+"?"+q.Encode(), nil, &out)
	if status == http.StatusNotFound {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if out.ParentFolderID == "" {
		return "", false, fmt.Errorf("graph: the lookup of message %s named no folder: %w", msgID, ErrUnreachable)
	}
	return out.ParentFolderID, true, nil
}

// FindMessage lists the folders that hold a message with this
// internetMessageId, at most findMessageCopies of them. One identity can sit
// in several folders, such as mail sent to oneself.
func (a *httpAPI) FindMessage(ctx context.Context, accessToken, internetMessageID string) ([]string, error) {
	q := url.Values{
		paramFilter: {internetMessageIDFilter(internetMessageID)},
		paramSelect: {"parentFolderId"},
		paramTop:    {strconv.Itoa(findMessageCopies)},
	}
	var out struct {
		Value []struct {
			ParentFolderID string `json:"parentFolderId"` //nolint:tagliatelle // Microsoft's wire format; must match to decode
		} `json:"value"`
	}
	if _, err := a.get(ctx, accessToken, a.base+"/me/messages?"+q.Encode(), nil, &out); err != nil {
		return nil, err
	}
	folders := make([]string, 0, len(out.Value))
	for _, m := range out.Value {
		if m.ParentFolderID == "" {
			return nil, fmt.Errorf("graph: the search for message %s named no folder: %w", internetMessageID, ErrUnreachable)
		}
		folders = append(folders, m.ParentFolderID)
	}
	return folders, nil
}

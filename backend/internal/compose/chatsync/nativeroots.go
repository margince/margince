// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func (p *Puller) pullNativeLink(ctx context.Context, session chatlinks.Session, link chatlinks.Link, reader chatwire.RootReader) (result error) {
	reason := ""
	defer func() {
		if result != nil && !errors.Is(result, errPullAgain) {
			reason, _ = nativeRootFailure(result)
		}
		result = errors.Join(result, p.Store.RecordScanResult(ctx, link.ID, reason))
	}()
	scan, err := p.Store.StartRootScan(ctx, link, p.Now())
	if err != nil {
		return err
	}
	var page chatwire.RootPage
	if link.RootID != "" {
		page = chatwire.RootPage{Roots: []chatwire.RootRef{{ID: link.RootID, Revision: "bound-root"}}}
	} else {
		since := time.Time{}
		if !link.BackfillRequested {
			since = link.CreatedAt
			if link.ReadSince != nil {
				since = *link.ReadSince
			}
		}
		if scan.Coverage != nil {
			since = *scan.Coverage
		}
		page, err = reader.Roots(ctx, link.ChannelSlug, since, scan.Cursor)
		if err != nil {
			return err
		}
	}
	if err := p.Store.RecordRootPage(ctx, link, scan, page); err != nil {
		return err
	}
	roots, err := p.Store.RootsToRefresh(ctx, link, p.Now().Add(-time.Hour))
	if err != nil {
		return err
	}
	for _, root := range roots {
		if err := p.refreshNativeRoot(ctx, session, link, root, reader); err != nil {
			reason, err = p.recordNativeRootFailure(ctx, link, root, err)
			if err != nil {
				return err
			}
		}
	}
	if page.Next != "" {
		return errPullAgain
	}
	return nil
}

func readNativeThread(ctx context.Context, reader chatwire.RootReader, channel, root string) ([]chatwire.Message, error) {
	messages := []chatwire.Message{}
	cursor := ""
	seen := map[string]bool{}
	runes := 0
	messageIDs := map[string]bool{}
	revision := ""
	for range 100 {
		page, err := reader.Thread(ctx, channel, root, cursor)
		if err != nil {
			return nil, err
		}
		if page.Revision != "" {
			if revision != "" && revision != page.Revision {
				return nil, chatwire.ErrUnreachable
			}
			revision = page.Revision
		}
		if page.Deleted {
			return nil, chatwire.ErrChannelGone
		}
		for _, message := range page.Messages {
			if message.TS == "" || messageIDs[message.TS] {
				return nil, chatwire.ErrUnreachable
			}
			messageIDs[message.TS] = true
			runes += utf8.RuneCountInString(message.Text)
			messages = append(messages, message)
			if runes > 120000 || len(messages) > 1000 {
				return nil, chatlinks.ErrThreadTooLong
			}
		}
		if page.Next == "" {
			slices.SortFunc(messages, func(a, b chatwire.Message) int {
				if compared := a.PostedAt.Compare(b.PostedAt); compared != 0 {
					return compared
				}
				return strings.Compare(a.TS, b.TS)
			})
			return messages, nil
		}
		if seen[page.Next] {
			return nil, chatwire.ErrUnreachable
		}
		seen[page.Next] = true
		cursor = page.Next
	}
	return nil, chatlinks.ErrThreadTooLong
}

func (p *Puller) refreshNativeRoot(ctx context.Context, session chatlinks.Session, link chatlinks.Link, root chatlinks.RootState, reader chatwire.RootReader) error {
	if root.Deleted {
		return p.withdrawNativeRoot(ctx, link, root, true)
	}
	messages, err := readNativeThread(ctx, reader, link.ChannelSlug, root.Root)
	if errors.Is(err, chatwire.ErrChannelGone) {
		return p.withdrawNativeRoot(ctx, link, root, true)
	}
	if err != nil {
		return err
	}
	digest, err := nativeContentDigest(messages)
	if err != nil {
		return err
	}
	if root.Digest != nil && *root.Digest == digest {
		return p.completeNativeRoot(ctx, link, root, digest, nil)
	}
	messages = eligibleNativeMessages(link, messages)
	if len(messages) == 0 {
		return p.completeNativeRoot(ctx, link, root, digest, nil)
	}
	record, err := readRecord(ctx, p.Pool, link)
	if err != nil {
		return err
	}
	reply, err := (summariser{lane: p.Lane, pool: p.Pool, restricted: p.restricted}).summarise(ctx, link, record, messages, link.RootID != "")
	if errors.Is(err, errNoModel) {
		return errModelDeferred
	}
	if err != nil {
		return err
	}
	if err := recheckNativeThread(ctx, reader, link.ChannelSlug, root.Root, digest); err != nil {
		return err
	}
	if !reply.Relevant && root.Activity == nil {
		return p.completeNativeRoot(ctx, link, root, digest, nil)
	}
	if !reply.Relevant {
		return p.withdrawNativeRoot(ctx, link, root, false)
	}
	keyHash := sha256.Sum256([]byte(link.ConnectionID.String() + "\x00" + link.ChannelSlug + "\x00" + root.Root))
	said := talkOf("root:"+hex.EncodeToString(keyHash[:]), chatlinks.ProviderChannelURL(link.Provider, session.TeamID, session.Address, link.ChannelSlug), messages)
	said.rootID = root.Root
	_, err = fileNote(ctx, filing{reconfirm: p.Reconfirm, pool: p.Pool, store: p.Store, acts: p.Activities, link: link, root: &root, digest: digest}, said, reply)
	return err
}

func (p *Puller) completeNativeRoot(ctx context.Context, link chatlinks.Link, root chatlinks.RootState, digest string, activity *ids.UUID) error {
	return database.WithTx(ctx, p.Pool, func(tx pgx.Tx) error { return p.Store.CompleteRootTx(ctx, tx, link, root, digest, activity) })
}

func (p *Puller) withdrawNativeRoot(ctx context.Context, link chatlinks.Link, root chatlinks.RootState, deleted bool) error {
	return database.WithTx(ctx, p.Pool, func(tx pgx.Tx) error {
		if err := p.Store.HoldLiveLinkTx(ctx, tx, link); err != nil {
			return err
		}
		if root.Activity != nil {
			_, err := p.Activities.ReviseChatSummaryTx(ctx, tx, ids.ActivityID{UUID: *root.Activity}, activities.ChatSummaryRevision{
				Subject: "Conversation source changed", Body: "This recap is no longer available because its source was removed.", Deleted: deleted, PreserveBody: !deleted,
				Metadata: map[string]any{"provider": link.Provider, "source_deleted": deleted, "source_changed": true, "connection_id": link.ConnectionID.String(), "link_id": link.ID.String(), "channel_id": link.ChannelSlug, "root_id": root.Root},
			})
			if err != nil {
				return err
			}
		}
		return p.Store.CompleteRootTx(ctx, tx, link, root, "deleted", nil)
	})
}

func nativeRootFailure(err error) (string, bool) {
	var limited *connector.RateLimitedError
	switch {
	case errors.Is(err, chatwire.ErrHistoryProfileNeedsReview):
		return "history_profile", true
	case errors.Is(err, chatwire.ErrTokenRejected), errors.Is(err, chatwire.ErrNotMember), errors.Is(err, chatwire.ErrChannelGone):
		return "access_lost", true
	case errors.As(err, &limited):
		return "rate_limited", true
	case errors.Is(err, errNoModel), errors.Is(err, errModelDeferred), ai.IsDeferral(err):
		return "model_unavailable", true
	case errors.Is(err, chatlinks.ErrThreadTooLong):
		return "source_too_long", false
	case errors.Is(err, apperrors.ErrConflict):
		return "source_changed", false
	default:
		return "source_unavailable", false
	}
}

func (p *Puller) recordNativeRootFailure(ctx context.Context, link chatlinks.Link, root chatlinks.RootState, failure error) (string, error) {
	classification, stop := nativeRootFailure(failure)
	retry := 5 * time.Minute
	var limited *connector.RateLimitedError
	if errors.As(failure, &limited) && limited.RetryAfter > retry {
		retry = limited.RetryAfter
	}
	if recorded := p.Store.RecordRootFailure(ctx, link, root, classification, p.Now().Add(retry)); recorded != nil {
		return classification, errors.Join(failure, recorded)
	}
	if stop {
		return classification, failure
	}
	return classification, nil
}

func eligibleNativeMessages(link chatlinks.Link, messages []chatwire.Message) []chatwire.Message {
	since := time.Time{}
	if !link.IncludeHistory {
		since = link.CreatedAt
	}
	since = readWindowStart(link, since)
	return slices.DeleteFunc(messages, func(message chatwire.Message) bool { return !judged(message) || message.PostedAt.Before(since) })
}

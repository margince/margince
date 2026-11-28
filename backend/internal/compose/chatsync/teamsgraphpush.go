// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/teams"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// TeamsGraphReceiver validates subscription notices before canonical channel reads.
type TeamsGraphReceiver struct {
	*Receiver
	Store *chatlinks.Store
	App   BotApp
}

//nolint:tagliatelle // Microsoft's provider protocol fixes these JSON field names.
type graphChatNotice struct {
	Subscription string `json:"subscriptionId"`
	Tenant       string `json:"tenantId"`
	ClientState  string `json:"clientState"`
	Resource     string `json:"resource"`
	Change       string `json:"changeType"`
	Lifecycle    string `json:"lifecycleEvent"`
}

type checkedGraphNotice struct {
	watch   chatlinks.ChannelWatch
	notice  graphChatNotice
	root    string
	deleted bool
}

// Receive validates provider notifications before queueing source synchronization.
func (r *TeamsGraphReceiver) Receive(ctx context.Context, d chatlinks.Delivery) (out crmcontracts.DispactChannelEventReceipt, result error) {
	ctx = principal.WithActor(ctx, chatLinksPrincipal)
	var envelope struct {
		Value []graphChatNotice `json:"value"`
	}
	if json.Unmarshal(d.Body, &envelope) != nil || !validGraphBatchSize(len(envelope.Value)) {
		return crmcontracts.DispactChannelEventReceipt{}, chatlinks.ErrEventMalformed
	}
	checked, err := r.checkGraphBatch(ctx, envelope.Value)
	if err != nil {
		return crmcontracts.DispactChannelEventReceipt{}, err
	}

	hash := sha256.Sum256(d.Body)
	revision := hex.EncodeToString(hash[:])
	if err := r.spend(ctx, "teams-graph|"+revision); errors.Is(err, chatlinks.ErrEventReplayed) {
		return receipt(crmcontracts.DispactChannelEventReceiptStatusAlreadyTaken), nil
	} else if err != nil {
		return crmcontracts.DispactChannelEventReceipt{}, err
	}
	defer func() {
		if result != nil {
			r.release(ctx, "teams-graph|"+revision)
		}
	}()

	for _, item := range checked {
		if item.notice.Lifecycle != "" {
			if err := r.Store.RecordWatchLifecycle(ctx, item.watch, item.notice.Lifecycle); err != nil {
				return crmcontracts.DispactChannelEventReceipt{}, err
			}
		} else if err := r.Store.InvalidateNativeRoot(ctx, item.watch.Connection, item.watch.Channel, chatwire.RootRef{ID: item.root, Revision: revision, Deleted: item.deleted}); err != nil {
			return crmcontracts.DispactChannelEventReceipt{}, err
		}
	}

	queued := map[string]bool{}
	for _, item := range checked {
		key := item.watch.Connection.String()
		if queued[key] {
			continue
		}
		if err := r.Pull(ctx, item.watch.Connection); err != nil {
			return crmcontracts.DispactChannelEventReceipt{}, err
		}
		queued[key] = true
	}
	return receipt(crmcontracts.DispactChannelEventReceiptStatusPullQueued), nil
}

func (r *TeamsGraphReceiver) checkGraphBatch(ctx context.Context, notices []graphChatNotice) ([]checkedGraphNotice, error) {
	app, err := r.App(ctx, chatlinks.ProviderTeams)
	if err != nil {
		return nil, chatlinks.ErrEventsUnavailable
	}
	checked := make([]checkedGraphNotice, 0, len(notices))
	for _, notice := range notices {
		item, known, err := r.checkGraphNotice(ctx, app, notice)
		if err != nil {
			return nil, err
		}
		if known {
			checked = append(checked, item)
		}
	}
	return checked, nil
}

func (r *TeamsGraphReceiver) checkGraphNotice(ctx context.Context, app capture.ConnectorApp, notice graphChatNotice) (checkedGraphNotice, bool, error) {
	watch, err := r.Store.WatchForNotification(ctx, notice.Subscription)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return checkedGraphNotice{}, false, nil
		}
		return checkedGraphNotice{}, false, errors.Join(chatlinks.ErrEventsUnavailable, err)
	}
	if watch.App != app.ClientID || watch.Tenant != app.Tenant || notice.Tenant != watch.Tenant {
		return checkedGraphNotice{}, false, chatlinks.ErrEventRejected
	}
	secret, err := r.Store.ChannelWatchSecret(ctx, watch)
	if err != nil {
		return checkedGraphNotice{}, false, errors.Join(chatlinks.ErrEventsUnavailable, err)
	}
	if subtle.ConstantTimeCompare([]byte(secret), []byte(notice.ClientState)) != 1 {
		return checkedGraphNotice{}, false, chatlinks.ErrEventRejected
	}
	root, isRoot, err := graphNoticeTarget(notice, watch.Channel)
	if err != nil {
		return checkedGraphNotice{}, false, err
	}
	return checkedGraphNotice{watch: watch, notice: notice, root: root, deleted: isRoot && notice.Change == "deleted"}, true, nil
}

func graphNoticeTarget(notice graphChatNotice, channel string) (string, bool, error) {
	root, isRoot, ok := teams.NotificationRoot(notice.Resource, channel)
	if notice.Lifecycle != "" {
		if notice.Lifecycle != "reauthorizationRequired" && notice.Lifecycle != "subscriptionRemoved" && notice.Lifecycle != "missed" {
			return "", false, chatlinks.ErrEventMalformed
		}
	} else if !ok || notice.Change != "created" && notice.Change != "updated" && notice.Change != "deleted" {
		return "", false, chatlinks.ErrEventRejected
	}
	return root, isRoot, nil
}

func validGraphBatchSize(size int) bool { return size > 0 && size <= 1000 }

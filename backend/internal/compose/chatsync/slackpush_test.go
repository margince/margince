// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type recordedRootNotice struct {
	connection ids.UUID
	channel    string
	root       chatwire.RootRef
}
type rootNoticeBoundary []recordedRootNotice

func (r *rootNoticeBoundary) InvalidateNativeRoot(_ context.Context, connection ids.UUID, channel string, root chatwire.RootRef) error {
	*r = append(*r, recordedRootNotice{connection: connection, channel: channel, root: root})
	return nil
}

func signedSlack(t *testing.T, body string, at time.Time) chatlinks.Delivery {
	t.Helper()
	d := chatlinks.Delivery{Timestamp: strconv.FormatInt(at.Unix(), 10), Body: []byte(body)}
	mac := hmac.New(sha256.New, []byte(pushSecret))
	if _, err := mac.Write([]byte("v0:" + d.Timestamp + ":" + body)); err != nil {
		t.Fatal(err)
	}
	d.Signature = "v0=" + hex.EncodeToString(mac.Sum(nil))
	return d
}

func TestSlackSignedDeletionIsBoundToInstallationAndNativeRoot(t *testing.T) {
	e := newPushEnv(t)
	roots := rootNoticeBoundary{}
	receiver := SlackReceiver{Receiver: e.receiver, Roots: &roots}
	ctx := context.Background()
	delivery := signedSlack(t, `{"type":"event_callback","team_id":"team-ours","event_id":"Ev-root-removed","event":{"type":"message","subtype":"message_deleted","channel":"C-one","deleted_ts":"123.000001","ts":"999.000001"}}`, pushNow)
	got, err := receiver.Receive(ctx, delivery)
	if err != nil || statusOf(got) != "pull_queued" || len(roots) != 1 || len(e.pulls) != 1 {
		t.Fatalf("valid notice not applied: %+v %v roots=%+v pulls=%v", got, err, roots, e.pulls)
	}
	if roots[0].connection != e.conn || roots[0].channel != "C-one" || roots[0].root.ID != "123.000001" || !roots[0].root.Deleted {
		t.Fatalf("lost deletion identity: %+v", roots[0])
	}
	got, err = receiver.Receive(ctx, delivery)
	if err != nil || statusOf(got) != "already_taken" || len(roots) != 1 || len(e.pulls) != 1 {
		t.Fatalf("replay was applied twice: %+v %v", got, err)
	}
}

func TestSlackRejectsForgedStaleAndForeignInstallationNotices(t *testing.T) {
	const body = `{"type":"event_callback","team_id":"team-ours","event_id":"Ev-message","event":{"type":"message","channel":"C-one","ts":"123.000001"}}`
	for _, name := range []string{"body changed", "signature missing", "expired", "future", "another team"} {
		t.Run(name, func(t *testing.T) {
			e := newPushEnv(t)
			roots := rootNoticeBoundary{}
			receiver := SlackReceiver{Receiver: e.receiver, Roots: &roots}
			delivery := signedSlack(t, body, pushNow)
			switch name {
			case "body changed":
				delivery.Body = []byte(body + " ")
			case "signature missing":
				delivery.Signature = ""
			case "expired":
				delivery = signedSlack(t, body, pushNow.Add(-6*time.Minute))
			case "future":
				delivery = signedSlack(t, body, pushNow.Add(6*time.Minute))
			case "another team":
				delivery = signedSlack(t, `{"type":"event_callback","team_id":"foreign-team","event_id":"Ev-message","event":{"type":"message","channel":"C-one","ts":"123.000001"}}`, pushNow)
			}
			_, err := receiver.Receive(context.Background(), delivery)
			if !errors.Is(err, chatlinks.ErrEventRejected) || len(roots) != 0 || len(e.pulls) != 0 || len(e.replays) != 0 {
				t.Fatalf("rejected notice took effect: %v roots=%+v pulls=%v replays=%v", err, roots, e.pulls, e.replays)
			}
		})
	}
}

func TestDispactNoticeUsesPermanentIdentityAfterChannelRename(t *testing.T) {
	e := newPushEnv(t)
	e.links.links[0].ChannelSlug = "immutable-channel"
	roots := rootNoticeBoundary{}
	e.receiver.Roots = &roots
	body := `{"type":"message.deleted","workspace_id":"team-ours","channel_slug":"renamed-channel","channel_id":"immutable-channel","message_id":"immutable-root","root_message_id":"immutable-root","event_id":"source-revision"}`
	got, err := e.receive(signed(t, pushSecret, "notice-delete", pushNow, body))
	if err != nil || statusOf(got) != "pull_queued" || len(roots) != 1 || roots[0].root.ID != "immutable-root" || !roots[0].root.Deleted {
		t.Fatalf("immutable deletion notice lost after rename: %+v %v roots=%+v", got, err, roots)
	}
}

func TestSlackSetupChallengeDoesNotRequireAnInstalledBotButStillChecksSignature(t *testing.T) {
	receiver := SlackReceiver{Receiver: &Receiver{Now: func() time.Time { return pushNow }}, SigningSecret: func(context.Context) (string, error) { return pushSecret, nil }}
	delivery := signedSlack(t, `{"type":"url_verification","challenge":"setup-proof"}`, pushNow)
	receipt, err := receiver.Receive(context.Background(), delivery)
	if err != nil || receipt.Challenge == nil || *receipt.Challenge != "setup-proof" {
		t.Fatalf("setup challenge refused: %+v %v", receipt, err)
	}
	delivery.Signature = "forged"
	if _, err := receiver.Receive(context.Background(), delivery); !errors.Is(err, chatlinks.ErrEventRejected) {
		t.Fatalf("unsigned setup accepted: %v", err)
	}
}

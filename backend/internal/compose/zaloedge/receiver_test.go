// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package zaloedge_test

// The production receiver's verify-then-decode order, without a database. The
// receiver here has no database and no queue, so a regression that stored or
// queued anything before the signature passed would fail these cases.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/compose/zaloedge"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type fixedEndpoints []capture.ZaloEndpoint

func (f fixedEndpoints) LiveEndpoints(context.Context) ([]capture.ZaloEndpoint, error) { return f, nil }

const (
	unitApp  = "1111222233334444"
	unitTime = "1764070000123"
	unitOA   = "9990001"
	unitKey  = "oa-secret-key"
)

func unitReceiver() *zaloedge.Receiver {
	return &zaloedge.Receiver{
		Endpoints: fixedEndpoints{{ConnectionID: ids.NewV7(), OAID: unitOA, AppID: unitApp, OASecretKey: unitKey}},
		Log:       slog.Default(),
	}
}

func unitSign(secret string, body []byte) string {
	sum := sha256.Sum256([]byte(unitApp + string(body) + unitTime + secret))
	return "mac=" + hex.EncodeToString(sum[:])
}

func unitMessage(text string) []byte {
	return []byte(fmt.Sprintf(`{"app_id":%q,"event_name":"user_send_text","timestamp":%q,"sender":{"id":"5550001"},"recipient":{"id":%q},"message":{"msg_id":"m1","text":%q}}`,
		unitApp, unitTime, unitOA, text))
}

func TestReceiveRefusesAnUnsignedOrMisSignedEventBeforeStoringAnything(t *testing.T) {
	body := unitMessage("hello")
	for name, signature := range map[string]string{
		"unsigned": "", "wrong key": unitSign("other", body), "tampered": unitSign(unitKey, unitMessage("other text")),
	} {
		if _, err := unitReceiver().Receive(context.Background(), body, signature); !errors.Is(err, capture.ErrChannelEventRejected) {
			t.Errorf("%s: got %v, want the rejection", name, err)
		}
	}
}

func TestReceiveRefusesABodyThatIsNoJSONWhenItIsUnsigned(t *testing.T) {
	if _, err := unitReceiver().Receive(context.Background(), []byte("not json"), unitSign(unitKey, []byte("not json"))); !errors.Is(err, capture.ErrChannelEventRejected) {
		t.Errorf("got %v, want the rejection: the signature needs the body's own app id and timestamp", err)
	}
}

func TestReceiveDecodesASignedEventAndSkipsOneThatIsNoMessage(t *testing.T) {
	follow := []byte(fmt.Sprintf(`{"app_id":%q,"event_name":"follow","timestamp":%q,"follower":{"id":"5550001"},"oa_id":%q}`, unitApp, unitTime, unitOA))
	receipt, err := unitReceiver().Receive(context.Background(), follow, unitSign(unitKey, follow))
	if err != nil || receipt.Stored != 0 || receipt.Skipped != 1 {
		t.Errorf("a signed follow = %+v, %v, want it read and skipped with nothing stored", receipt, err)
	}
	other := []byte(fmt.Sprintf(`{"app_id":%q,"event_name":"user_send_text","timestamp":%q,"sender":{"id":"5550001"},"recipient":{"id":"another-oa"},"message":{"msg_id":"m1","text":"x"}}`, unitApp, unitTime))
	receipt, err = unitReceiver().Receive(context.Background(), other, unitSign(unitKey, other))
	if err != nil || receipt.Stored != 0 || receipt.Skipped != 1 {
		t.Errorf("a signed message for another OA = %+v, %v, want it skipped before any store", receipt, err)
	}
}

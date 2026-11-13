// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dispactapp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Tools are the tools the Dispact agent calls, agreed with Dispact. The first
// five are router actions served as their own verbs. file_channel_message is
// listed once its verb is registered; until then Registry.Fixed leaves it out.
var Tools = []string{
	"channel_link", "link_channel", "unlink_channel", "set_channel_verbosity", "file_channel_message",
	"find_records", "read_record", "read_context", "whoami",
}

// NoSeatCode is the answer to a caller the app vouches for who is no member.
const NoSeatCode = "no_margince_seat"

// noSeat refuses every request but the handshake and the tool list.
type noSeat struct{}

func (noSeat) Error() string { return "dispactapp: the caller has no Margince member" }

func (noSeat) WriteRefusal(w http.ResponseWriter, r *http.Request) {
	httperr.Write(w, r, &httperr.DetailedError{
		Status: http.StatusForbidden,
		Code:   NoSeatCode,
		Detail: "Your chat address belongs to no Margince member. Ask a Margince admin to invite you with that address.",
	})
}

// PaceKey is the key a call is paced under. A call token is new on every
// call, so an app call is paced by its passport; any other call keeps key.
func PaceKey(ctx context.Context, key string) string {
	p, ok := principal.Actor(ctx)
	if !ok || !agents.IsAppCall(ctx) {
		return key
	}
	return "app:" + p.PassportID.String()
}

// RedisReplays holds token ids in Redis, shared by every API replica.
type RedisReplays struct{ RDB *redis.Client }

// replayKeyPrefix namespaces the token ids beside the bus's own keys.
const replayKeyPrefix = "gw:dispact-jti:"

func (r RedisReplays) FirstUse(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	sum := sha256.Sum256([]byte(key))
	return r.RDB.SetNX(ctx, replayKeyPrefix+hex.EncodeToString(sum[:]), 1, ttl).Result()
}

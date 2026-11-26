// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"github.com/redis/go-redis/v9"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/platform/lastseen"
	"github.com/margince/margince/backend/internal/platform/replayguard"
)

// redisSurfaceOptions are the surfaces that count or remember in this role's
// Redis. The agent volume meter counts there. Dispact's push receiver holds each
// notice id there once, with the moment of the last valid notice. The worker's
// sweep reads that moment to slow its polling.
func redisSurfaceOptions(rdb *redis.Client, volumeMeter *agentvolume.Meter) []compose.Option {
	return append(agentVolumeOptions(volumeMeter), compose.WithChatEvents(replayguard.New(rdb), lastseen.New(rdb)))
}

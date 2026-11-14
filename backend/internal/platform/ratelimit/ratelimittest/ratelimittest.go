// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package ratelimittest gives a test a limiter registry whose shared count is
// out. A caller above the platform tier proves with it how its limiter fails,
// without importing a Redis client itself.
package ratelimittest

import (
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/margince/margince/backend/internal/platform/ratelimit"
)

// Down answers a registry bound to a Redis that refuses every connection, as
// one that cannot be reached does.
func Down() *ratelimit.Registry {
	return ratelimit.Shared(redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1, DialTimeout: 100 * time.Millisecond}))
}

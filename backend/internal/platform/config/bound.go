// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

import (
	"context"
	"sync/atomic"
)

// A composition puts the configuration its program loaded on every context it
// serves. Package-level readers of a stored setting, called from many modules
// with only a transaction, read the boot locks from there. A program also
// binds it once, at boot, for the work that starts from a bare context.
var bound atomic.Pointer[Config]

// Bind makes c the configuration this process loaded.
func Bind(c *Config) { bound.Store(c) }

type boundKey struct{}

// WithBound is ctx serving the program that loaded c.
func WithBound(ctx context.Context, c *Config) context.Context {
	return context.WithValue(ctx, boundKey{}, c)
}

// Bound is the configuration of the program ctx serves, else the one this
// process bound, else nil. A nil Config locks nothing.
func Bound(ctx context.Context) *Config {
	if c, ok := ctx.Value(boundKey{}).(*Config); ok && c != nil {
		return c
	}
	return bound.Load()
}

// serving is c with the boot values of the program ctx serves, when c is a
// view over the database and ctx carries a configuration. A composition builds
// its views before any context exists, and each call reads its own program's.
func (c *Config) serving(ctx context.Context) *Config {
	if c == nil || c.db == nil {
		return c
	}
	program, ok := ctx.Value(boundKey{}).(*Config)
	if !ok || program == nil {
		return c
	}
	next := *program
	next.db, next.sealer = c.db, c.sealer
	return &next
}

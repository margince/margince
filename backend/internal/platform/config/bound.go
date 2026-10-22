// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

import "sync/atomic"

// A program binds the configuration it loaded once, at boot. Package-level
// readers of a stored setting, called from many modules with only a
// transaction, read the boot locks through it.
var bound atomic.Pointer[Config]

// Bind makes c the configuration this process loaded.
func Bind(c *Config) { bound.Store(c) }

// Bound is the configuration this process loaded, or nil before a program
// binds one. A nil Config locks nothing.
func Bound() *Config { return bound.Load() }

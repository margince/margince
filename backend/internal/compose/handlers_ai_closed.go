// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "regexp"

// tierRoutingPath is a tier's or the embeddings lane's `routing` block, which
// the routing document's own decoder judges and reports key by key in-band, so
// the closed decode leaves it to that layer.
var tierRoutingPath = regexp.MustCompile(`^(tiers\.[^.]+|embeddings)\.routing(\.|$)`)

func routingBlockIsOwned(path string) bool { return tierRoutingPath.MatchString(path) }

// taskOverrideTimeoutPath is one task's timeout in a task-override body.
// ai.TaskOverridesFromWire holds both bounds and lists every refused path in
// one 422, which the settings screen shows field by field.
var taskOverrideTimeoutPath = regexp.MustCompile(`^[^.]+\.(decision|attempt)_timeout_ms$`)

func taskOverrideBoundIsOwned(path string) bool { return taskOverrideTimeoutPath.MatchString(path) }

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package webhooks is the governed outbound integration surface. It holds the
// tenant's webhook_subscription rows and a delivery worker.
//
// The worker fans matching domain events from the event bus out to each target
// URL as an HMAC-SHA256-signed HTTP POST. It retries with exponential backoff
// and parks a failed delivery in a dead-letter store that can be replayed.
//
// The surface is first-party: subscriptions live in the workspace. It is not a
// third-party app marketplace and not an inbound receiver. A subscription's
// signing secret is sealed at rest (cipher.go) and returned once, at create or
// rotate.
//
// Tables owned: docs/reference/entity-model/webhooks.md.
package webhooks

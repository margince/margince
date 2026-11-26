// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package events

// OutboxEventIDSQL reads the envelope identity; the outbox row has its own ID.
// alias is a compile-time table alias, never input from a request.
func OutboxEventIDSQL(alias string) string { return alias + ".envelope->>'event_id'" }

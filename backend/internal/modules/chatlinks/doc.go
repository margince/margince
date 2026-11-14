// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package chatlinks links a deal or a project to one channel of the team chat
// and keeps the ledger of what Margince posted there. Handlers→Store.
//
// The engine that turns events into posts lives in compose/chatsync, because
// it crosses activities, deals and projects. This package owns the link, the
// post ledger, and the chat client it acts through.
//
// Writes here are audit-only. The closed event catalog has no verb for a chat
// link, and every reader reads the tables again.
//
// Tables owned: docs/reference/entity-model/chatlinks.md.
package chatlinks

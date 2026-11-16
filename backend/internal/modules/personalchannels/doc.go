// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package personalchannels lets a member bring their own chats and files into
// Margince and pull their replies back out, through a client they run (an AI
// assistant over MCP, or their own program with a passport). Margince never
// reads a chat network itself. Handlers→Store.
package personalchannels

// Provider is the one core channel provider these messages travel on. The
// network a message came from (whatsapp, zalo, …) is data on the message, so a
// new network needs no release.
const Provider = "personal_channels"

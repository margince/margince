// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package chatedge is what every company chat network's webhook edge shares.
// Storing a delivered message and queuing its ingest job is one step, and filing
// a stored message is one worker. A network supplies only how it verifies a
// delivery, how it splits one into messages and how it decodes a stored one.
package chatedge

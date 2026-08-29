// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package notices owns the durable informational notice: a line addressed to
// one person that a system flow needed them to see — an automation's notify
// firing, the lead-SLA's escalation notice — with its own read-state. The
// row IS the delivery transport: creating one is what "notified" means here,
// which is what lets an engine record a notify action successful without
// claiming a channel this repo does not have. Nothing here asks for a
// decision; a notice with a verb to perform belongs to approvals.
//
// Tables owned: notice
package notices

// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package openchannel is a connector demonstrated end to end: a member opens a
// signed, session-less edge, a remote system POSTs to it with no session at
// all, and what arrives is queued under that member's name for the CRM to act
// on later.
//
// WHAT MAKES IT A CONNECTOR RATHER THAN A WEBHOOK. The core owns admission and
// this unit owns meaning, and the split is the whole design. Before Handle is
// called the core has already refused an undeclared slug, capped the body,
// spent both rate budgets, bounded the timestamp and resolved the
// installation's workspace — everything decidable WITHOUT a secret. What is
// left is the part the core cannot do, because the secret lives in this unit's
// own namespace and the core has no way to read it: resolve the slug to its
// owner, fetch that owner's secret, verify the signature, and record what
// arrived.
//
// The anonymous edge carries NO authority. Its principal is a bare connector
// with empty permissions, so the only honest thing a verified request can buy
// is a row in a queue. Everything the payload eventually MEANS is decided
// later, under the owner's own live authority — which is why receiving and
// acting are two steps here and not one.
//
// Tables owned:
//
//   - ext_openchannel_endpoint — one row per opened edge: which declared slug
//     it is, whose consent stands behind it, where its replies go, and the
//     traffic counters a screen renders. It deliberately does NOT hold the
//     signing secret.
//   - ext_openchannel_inbound — the received-but-not-yet-ingested queue, keyed
//     for replay by (endpoint_id, nonce). It keeps the body verbatim, because
//     the body is what the signature covered.
//
// The signing secret is in the unit's USER-scoped secret namespace, under the
// key this unit declares. It is shown to the member once, at the moment it is
// minted, and no operation returns it again — a credential a surface can read
// back is one every holder of that surface's RBAC object holds.
//
// NOTHING about this unit's GOVERNANCE is repeated in Go: api/crm.yaml holds
// each operation's tier, scope, RBAC object, prose and schemas, and the inbound
// declaration in openchannel.go is what an operator reads to see that this unit
// has an anonymous edge at all.
package openchannel

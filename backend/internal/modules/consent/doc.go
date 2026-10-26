// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package consent owns per-purpose consent (A22/ADR-0011, data-model
// §3.4): the purpose catalog, each contact's current state, the
// append-only proof log — and the default-deny suppression gate that
// outbound surfaces consult before anything leaves the workspace. The
// gate answers per PURPOSE: a marketing grant never authorizes a
// profiling use; unknown and withdrawn both block.
//
// Tables owned: docs/reference/entity-model/consent.md.
//
// withdrawal_credential is the withdrawal-only link that outlives a preference
// token. It can stop mail and can neither read a consent state nor grant, so a
// two-year life is safe. communication_instruction is a named human's decision
// that one refused message goes out anyway. It sits beside the refusal and never
// replaces it with a grant nobody made. communication_override is a rep's
// standing vouch that outranks a machine-level refusal for one contact and
// category. It never outranks a subject-level stop. communication_review is what a refused send leaves behind, so the work
// is visible and the message can resume instead of being retyped.
// lead_privacy_notice, the record of a website lead's notice, names nobody.
// privacy_request_proof is what a closed request did. It is kept for three
// years past the year it closed in.
//
// Consumers (activities' send path) declare a one-method authority interface.
// The composition root injects this module's Gate and unsubscribe linker, so
// consent never becomes an import edge between siblings.
package consent

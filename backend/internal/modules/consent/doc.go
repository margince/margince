// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package consent owns per-purpose consent (A22/ADR-0011, data-model
// §3.4): the purpose catalog, each person's current state, the
// append-only proof log — and the default-deny suppression gate that
// outbound surfaces consult before anything leaves the workspace. The
// gate answers per PURPOSE: a marketing grant never authorizes a
// profiling use; unknown and withdrawn both block.
//
// Tables owned: consent_purpose, person_consent, consent_event,
// consent_doi_token, consent_qualifying_event, consent_existing_customer_flag,
// preference_token (the buyer-facing preference center's token→tenant
// resolver, B-E11.32), confirm_token, person_confirm_submission,
// data_subject_request, communication_decision, communication_basis,
// communication_suppression. Consumers (activities' send path) declare a
// one-method authority interface; the composition root injects this module's
// Gate and unsubscribe linker — consent never becomes an import edge between
// siblings.
package consent

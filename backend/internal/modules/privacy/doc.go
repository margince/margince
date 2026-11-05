// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package privacy owns the GDPR engines (ADR-0011): right-to-erasure
// (Art. 17), subject-access assembly (Art. 15) and the nightly
// retention evaluator. The DSR case queue (data_subject_request rows +
// their HTTP surface) stays in consent; this module is the machinery a
// fulfilled request executes — the composition root injects the Eraser
// into consent's DSR handlers, so the two never become an import edge.
//
// Tables owned: docs/reference/entity-model/privacy.md.
//
// Erasure and retention also write rows this module does not own. They reach
// contact, lead, activity, deal, embedding, raw_capture, approval and
// workflow_run, and the Deal Room's participant, session and engagement rows. A data-subject
// obligation must reach every store that holds the subject, in one transaction
// per record. Routing each purge through its owning module's API would trade
// that atomicity, which is the guarantee, for boundary hygiene. Each such write
// is ratified per table in backend/gates/tableownership_test.go.
//
// Imports shared + platform only; never a sibling module. Retention
// floors come from the compiled-in jurisdiction packs via
// shared/ports/jurisdiction.
package privacy
